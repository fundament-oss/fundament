import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
  untracked,
  OnInit,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerEye, tablerDatabaseOff } from '@ng-icons/tabler-icons';
import KubeClusterContextService from '../kube-cluster-context.service';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { ConfigService } from '../../config.service';
import OrganizationContextService from '../../organization-context.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, AdditionalPrinterColumn, KubeResource } from '../types';
import {
  resolveJsonPath,
  formatColumnValue,
  getListColumns,
  kindToLabel,
} from '../crd-schema.utils';

function buildDetailLink(resource: KubeResource): string[] {
  return ['.', resource.metadata.name];
}

function buildCellValue(resource: KubeResource, col: AdditionalPrinterColumn): string {
  const fullObj = {
    metadata: resource.metadata,
    spec: resource.spec,
    status: resource.status ?? {},
  };
  const value = resolveJsonPath(fullObj, col.jsonPath);
  return formatColumnValue(value, col.type);
}

@Component({
  selector: 'app-resource-list',
  imports: [RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerEye,
      tablerDatabaseOff,
    }),
  ],
  templateUrl: './resource-list.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceListComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private titleService = inject(TitleService);

  protected clusterContext = inject(KubeClusterContextService);

  private configService = inject(ConfigService);

  private orgContext = inject(OrganizationContextService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  resources = signal<KubeResource[]>([]);

  columns = computed<AdditionalPrinterColumn[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return getListColumns(crd.additionalPrinterColumns).filter(
      (col) => col.name !== 'Name' && col.name !== 'Age',
    );
  });

  kindLabel = computed(() => {
    const crd = this.crdDef();
    if (crd) return kindToLabel(crd.kind);

    // Fallback: look up the menu entry by plural to get the proper PascalCase CRD name
    const plugin = this.plugin();
    const resourceKind = this.resourceKind();
    if (plugin) {
      const allItems = [...(plugin.menu.organization ?? []), ...(plugin.menu.project ?? [])];
      const item = allItems.find((i) => i.plural === resourceKind);
      if (item) return kindToLabel(item.crd);
    }
    return kindToLabel(resourceKind);
  });

  constructor() {
    effect(() => {
      this.titleService.setTitle(this.kindLabel());
    });

    effect(() => {
      const pluginName = this.pluginName();
      const resourceKind = this.resourceKind();
      const clusterId = this.clusterContext.selectedClusterId();
      if (pluginName && resourceKind && clusterId) {
        untracked(() => {
          this.loadCrdsAndResources(clusterId);
        });
      }
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.clusterContext.loadClusters();
    } catch {
      this.errorMessage.set('Failed to load clusters.');
    }
  }

  onClusterChange(clusterId: string): void {
    this.clusterContext.onClusterChange(clusterId);
  }

  private async loadCrdsAndResources(clusterId: string): Promise<void> {
    const orgId = this.orgContext.currentOrganizationId();
    if (!orgId) return;

    const orgApiUrl = this.configService.getConfig().organizationApiUrl;
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.crdDef.set(undefined);
    this.resources.set([]);

    try {
      await this.registry.loadCrdsForPlugin(this.pluginName(), clusterId, orgApiUrl, orgId);
      const crd = this.registry.getCrdByPlural(this.pluginName(), this.resourceKind());
      this.crdDef.set(crd);

      if (crd) {
        await this.store.loadResources(this.pluginName(), crd, clusterId, orgApiUrl, orgId);
        this.resources.set(this.store.listResources(this.pluginName(), crd.kind, clusterId));
      }
    } catch (err) {
      this.errorMessage.set(`Failed to load resources: ${err}`);
    } finally {
      this.isLoading.set(false);
    }
  }

  detailLink = buildDetailLink;

  formatCell = buildCellValue;
}
