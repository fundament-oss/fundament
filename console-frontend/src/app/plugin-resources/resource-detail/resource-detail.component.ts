import { Component, ChangeDetectionStrategy, inject, computed, effect } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerArrowLeft } from '@ng-icons/tabler-icons';
import FieldRendererComponent from '../field-renderers/field-renderer.component';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, KubeResource, CrdPropertySchema } from '../types';
import { toDateValue, toSimpleValue, fieldNameToLabel } from '../crd-schema.utils';

function checkIsWideField(schema: CrdPropertySchema): boolean {
  return (
    schema.type === 'array' || schema.type === 'object' || (schema.description?.length ?? 0) > 100
  );
}

function checkIsWideStatusField(key: string, value: unknown): boolean {
  return key === 'conditions' || Array.isArray(value) || typeof value === 'object';
}

function checkIsConditionsField(key: string, value: unknown): boolean {
  return key === 'conditions' && Array.isArray(value);
}

function toArray(val: unknown): unknown[] {
  return Array.isArray(val) ? val : [];
}

function toRecord(val: unknown): Record<string, unknown> {
  return (val as Record<string, unknown>) ?? {};
}

@Component({
  selector: 'app-resource-detail',
  imports: [RouterLink, NgIcon, FieldRendererComponent],
  viewProviders: [provideIcons({ tablerArrowLeft })],
  templateUrl: './resource-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceDetailComponent {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private titleService = inject(TitleService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  private resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

  crdDef = computed<ParsedCrd | undefined>(() =>
    this.registry.getCrdByPlural(this.pluginName(), this.resourceKind()),
  );

  resource = computed<KubeResource | undefined>(() => {
    const crd = this.crdDef();
    if (!crd) return undefined;
    return this.store.getResource(this.pluginName(), crd.kind, this.resourceId());
  });

  fieldGroups = computed(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    const fields = Object.entries(crd.specSchema.properties) as [string, CrdPropertySchema][];
    return [{ name: 'Configuration', fields }];
  });

  statusFields = computed<[string, unknown][]>(() => {
    const r = this.resource();
    if (!r?.status) return [];
    return Object.entries(r.status);
  });

  constructor() {
    effect(() => {
      const r = this.resource();
      this.titleService.setTitle(r?.metadata.name);
    });
  }

  readonly listLink = ['..'];

  formatLabel = fieldNameToLabel;

  formatDateValue = toDateValue;

  formatSimpleValue = toSimpleValue;

  isWideField = checkIsWideField;

  isWideStatusField = checkIsWideStatusField;

  isConditionsField = checkIsConditionsField;

  asArray = toArray;

  asRecord = toRecord;

  getSpecValue(fieldName: string): unknown {
    return this.resource()?.spec[fieldName] ?? null;
  }
}
