import { Component, ChangeDetectionStrategy, inject, signal, computed } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerEye,
  tablerTrash,
  tablerAlertTriangle,
  tablerDatabaseOff,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../../modal/modal.component';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { ToastService } from '../../toast.service';
import type {
  PluginDefinition,
  ParsedCrd,
  PluginMenuItem,
  AdditionalPrinterColumn,
  KubeResource,
} from '../types';
import {
  resolveJsonPath,
  formatColumnValue,
  getListColumns,
  resolveStatusBadge,
} from '../crd-schema.utils';

function buildCreateLink(): string[] {
  return ['.', 'create'];
}

function buildDetailLink(resource: KubeResource): string[] {
  return ['.', resource.metadata.uid];
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
  standalone: true,
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerEye,
      tablerTrash,
      tablerAlertTriangle,
      tablerDatabaseOff,
    }),
  ],
  templateUrl: './resource-list.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceListComponent {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private toastService = inject(ToastService);

  private pluginName = this.route.snapshot.paramMap.get('pluginName') ?? '';

  private resourceKind = this.route.snapshot.paramMap.get('resourceKind') ?? '';

  plugin = computed<PluginDefinition | undefined>(() => this.registry.getPlugin(this.pluginName));

  crdDef = computed<ParsedCrd | undefined>(() =>
    this.registry.getCrdByPlural(this.pluginName, this.resourceKind),
  );

  menuItem = computed<PluginMenuItem | undefined>(() => {
    const p = this.plugin();
    const crd = this.crdDef();
    if (!p || !crd) return undefined;
    const allItems = [...(p.menu.organization ?? []), ...(p.menu.project ?? [])];
    return allItems.find((item) => item.crd === crd.kind);
  });

  columns = computed<AdditionalPrinterColumn[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return getListColumns(crd.additionalPrinterColumns).filter(
      (col) => col.name !== 'Name' && col.name !== 'Age',
    );
  });

  resources = computed<KubeResource[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return this.store.listResources(this.pluginName, crd.kind);
  });

  showDeleteModal = signal(false);

  pendingDeleteUid = signal('');

  pendingDeleteName = signal('');

  createLink = buildCreateLink;

  detailLink = buildDetailLink;

  formatCell = buildCellValue;

  getStatusBadge(
    resource: KubeResource,
    col: AdditionalPrinterColumn,
  ): { badge: string; label: string } | undefined {
    const p = this.plugin();
    const crd = this.crdDef();
    if (!p?.uiHints || !crd) return undefined;

    const hints = p.uiHints[crd.kind];
    if (!hints?.statusMapping) return undefined;

    if (col.jsonPath !== hints.statusMapping.jsonPath) return undefined;

    return resolveStatusBadge(resource, hints.statusMapping);
  }

  openDeleteModal(resource: KubeResource): void {
    this.pendingDeleteUid.set(resource.metadata.uid);
    this.pendingDeleteName.set(resource.metadata.name);
    this.showDeleteModal.set(true);
  }

  confirmDelete(): void {
    const crd = this.crdDef();
    if (!crd) return;
    this.store.deleteResource(this.pluginName, crd.kind, this.pendingDeleteUid());
    this.showDeleteModal.set(false);
    this.toastService.show(`${crd.singular} deleted`, 'success');
  }
}
