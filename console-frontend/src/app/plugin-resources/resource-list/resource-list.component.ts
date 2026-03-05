import { Component, ChangeDetectionStrategy, inject, computed, effect } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerEye, tablerDatabaseOff } from '@ng-icons/tabler-icons';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, AdditionalPrinterColumn, KubeResource } from '../types';
import {
  resolveJsonPath,
  formatColumnValue,
  getListColumns,
  kindToLabel,
} from '../crd-schema.utils';

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
export default class ResourceListComponent {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private titleService = inject(TitleService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  crdDef = computed<ParsedCrd | undefined>(() =>
    this.registry.getCrdByPlural(this.pluginName(), this.resourceKind()),
  );

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
    return this.store.listResources(this.pluginName(), crd.kind);
  });

  kindLabel = computed(() => {
    const crd = this.crdDef();
    return crd ? kindToLabel(crd.kind) : 'Resources';
  });

  constructor() {
    effect(() => {
      this.titleService.setTitle(this.kindLabel());
    });
  }

  detailLink = buildDetailLink;

  formatCell = buildCellValue;
}
