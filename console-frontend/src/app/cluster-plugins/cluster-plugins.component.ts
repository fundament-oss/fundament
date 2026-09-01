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
import { CLUSTER, PLUGIN } from '../../connect/tokens';
import { fetchClusterDetails, getStatusLabel } from '../utils/cluster-status';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { ListPluginsRequestSchema, type PluginSummary } from '../../generated/v1/plugin_pb';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import type { PluginInstallationItem } from '../plugin-resources/types';
import SheetSyncDirective from '../sheet-sync.directive';
import PageNavService from '../page-nav.service';

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

      await Promise.all([
        ...toInstall.map((p) =>
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
}
