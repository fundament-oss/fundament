import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ViewChild,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, ActivatedRoute } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { CLUSTER, INSTALL, PLUGIN } from '../../connect/tokens';
import { fetchClusterDetails, getStatusLabel } from '../utils/cluster-status';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import fetchPluginConfigSchema from '../plugin-installation/plugin-config-schema';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import type { PluginInstallationItem } from '../plugin-resources/types';
import SheetSyncDirective from '../sheet-sync.directive';
import PageNavService from '../page-nav.service';

import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

@Component({
  selector: 'app-cluster-plugins',
  imports: [SharedPluginsFormComponent, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-plugins.component.html',
})
export default class ClusterPluginsComponent implements OnInit {
  private pageNav = inject(PageNavService);

  @ViewChild(SharedPluginsFormComponent) pluginsForm!: SharedPluginsFormComponent;

  private titleService = inject(TitleService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private client = inject(CLUSTER);

  private pluginClient = inject(PLUGIN);

  private installClient = inject(INSTALL);

  private pluginInstallationService = inject(PluginInstallationService);

  private clusterId = '';

  private allPlugins: PluginSummary[] = [];

  private currentInstallations: PluginInstallationItem[] = [];

  errorMessage = signal<string | null>(null);

  isSubmitting = signal(false);

  isLoading = signal(true);

  loadFailed = signal(false);

  currentPluginIds = signal<string[]>([]);

  clusterName = signal<string | null>(null);

  /** Falls back to the bare noun while the cluster name is still loading, so the
   *  title bar never shows a dangling "Plugins for". */
  protected pageTitle = computed(() => {
    const name = this.clusterName();
    return name ? `Plugins for ${name}` : 'Plugins';
  });

  protected clusterStatus = signal<ClusterStatus>(ClusterStatus.UNSPECIFIED);

  protected isClusterRunning = computed(() => this.clusterStatus() === ClusterStatus.RUNNING);

  /** The state where this sheet has nothing to offer: the status is in, and it
   *  is not running. Not while loading, when the status is still UNSPECIFIED. */
  protected notRunning = computed(() => !this.isLoading() && !this.isClusterRunning());

  protected readonly getStatusLabel = getStatusLabel;

  constructor() {
    this.titleService.setTitle('Plugins');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  ngOnInit() {
    this.load();
  }

  /** Nothing here renders before this resolves. The status arrives with it, and
   *  a status-dependent warning drawn on the default of UNSPECIFIED flashes a
   *  banner on open that is gone before it can be read. */
  async load() {
    this.isLoading.set(true);
    this.loadFailed.set(false);
    try {
      const [, pluginsResponse, installations] = await Promise.all([
        fetchClusterDetails(this.client, this.clusterId).then(({ name, status }) => {
          this.clusterName.set(name);
          this.clusterStatus.set(status);
        }),
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
        this.pluginInstallationService.listInstallations(this.clusterId).catch(() => []),
      ]);

      this.allPlugins = pluginsResponse.plugins;
      this.currentInstallations = installations;

      const installedNames = new Set(installations.map((i) => i.spec.definitionRef.pluginName));
      this.currentPluginIds.set(
        this.allPlugins.filter((p) => installedNames.has(p.name)).map((p) => p.id),
      );
    } catch {
      this.loadFailed.set(true);
    } finally {
      this.isLoading.set(false);
    }
  }

  async onFormSubmit(data: { preset: string; plugins: string[] }) {
    if (this.isSubmitting() || !this.isClusterRunning()) return;
    this.isSubmitting.set(true);
    this.errorMessage.set(null);

    try {
      const newPlugins = data.plugins
        .map((id) => this.allPlugins.find((p) => p.id === id))
        .filter((p): p is PluginSummary => !!p);

      const currentNames = new Set(
        this.currentInstallations.map((i) => i.spec.definitionRef.pluginName),
      );
      const newNames = new Set(newPlugins.map((p) => p.name));

      const toInstall = newPlugins.filter((p) => !currentNames.has(p.name));
      const toUninstall = this.currentInstallations.filter(
        (i) => !newNames.has(i.spec.definitionRef.pluginName),
      );

      // This bulk form has no per-plugin config step (the install modal on
      // the plugins page does), so a plugin whose definition declares
      // required config would only ever reconcile to Failed from here. Fail
      // closed per plugin — a schema fetch error counts as requiring config —
      // while the rest of the submit (other installs, all uninstalls) still
      // goes through.
      const checked = await Promise.all(
        toInstall.map(async (p) => ({
          plugin: p,
          needsConfig: await this.requiresConfigForm(p).catch(() => true),
        })),
      );
      const blocked = checked.filter((c) => c.needsConfig).map((c) => c.plugin);
      const installable = checked.filter((c) => !c.needsConfig).map((c) => c.plugin);

      await Promise.all([
        ...installable.map((p) =>
          this.pluginInstallationService.installPlugin(
            this.clusterId,
            p.organizationName,
            p.name,
            p.pluginVersion,
            p.definitionHash,
          ),
        ),
        ...toUninstall.map((i) =>
          this.pluginInstallationService.uninstallPlugin(this.clusterId, i.metadata.name),
        ),
      ]);

      if (blocked.length > 0) {
        this.errorMessage.set(
          `${blocked.map((p) => p.displayName || p.name).join(', ')} must be configured at install time — install from the plugins page instead. Everything else was applied.`,
        );
        await this.load();
        return;
      }

      this.pageNav.goTo(`/clusters/${this.clusterId}`);
    } catch {
      this.errorMessage.set('Failed to update cluster plugins');
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.pageNav.goTo(`/clusters/${this.clusterId}`);
  }

  // True when the pinned definition declares required config keys. The shared
  // fetch throws when the catalog cannot say (schema unavailable), and both
  // that and a fetch error are caught per plugin in onFormSubmit, failing
  // closed like the install modal does.
  private async requiresConfigForm(plugin: PluginSummary): Promise<boolean> {
    const schema = await fetchPluginConfigSchema(
      this.installClient,
      plugin.organizationName,
      plugin.name,
      plugin.pluginVersion,
    );
    return schema.some((entry) => entry.required);
  }
}
