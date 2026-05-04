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

@Component({
  selector: 'app-cluster-plugins',
  imports: [SharedPluginsFormComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-plugins.component.html',
})
export default class ClusterPluginsComponent implements OnInit {
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

  currentPluginIds = signal<string[]>([]);

  clusterName = signal<string | null>(null);

  protected clusterStatus = signal<ClusterStatus>(ClusterStatus.UNSPECIFIED);

  protected isClusterRunning = computed(() => this.clusterStatus() === ClusterStatus.RUNNING);

  protected readonly getStatusLabel = getStatusLabel;

  constructor() {
    this.titleService.setTitle('Cluster plugins');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async ngOnInit() {
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

    const installedNames = new Set(installations.map((i) => i.spec.pluginName));
    this.currentPluginIds.set(
      this.allPlugins.filter((p) => installedNames.has(p.name)).map((p) => p.id),
    );
  }

  async onFormSubmit(data: { preset: string; plugins: string[] }) {
    if (this.isSubmitting() || !this.isClusterRunning()) return;
    this.isSubmitting.set(true);
    this.errorMessage.set(null);

    try {
      const newPlugins = data.plugins
        .map((id) => this.allPlugins.find((p) => p.id === id))
        .filter((p): p is PluginSummary => !!p);

      const currentNames = new Set(this.currentInstallations.map((i) => i.spec.pluginName));
      const newNames = new Set(newPlugins.map((p) => p.name));

      const toInstall = newPlugins.filter((p) => !currentNames.has(p.name));
      const toUninstall = this.currentInstallations.filter((i) => !newNames.has(i.spec.pluginName));

      await Promise.all([
        ...toInstall.map((p) =>
          this.pluginInstallationService.installPlugin(this.clusterId, p.name, p.image),
        ),
        ...toUninstall.map((i) =>
          this.pluginInstallationService.uninstallPlugin(this.clusterId, i.metadata.name),
        ),
      ]);

      this.router.navigate(['/clusters', this.clusterId]);
    } catch {
      this.errorMessage.set('Failed to update cluster plugins');
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }
}
