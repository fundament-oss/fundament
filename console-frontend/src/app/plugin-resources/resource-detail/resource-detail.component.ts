import {
  Component,
  ChangeDetectionStrategy,
  inject,
  signal,
  computed,
  effect,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerAlertTriangle,
  tablerArrowLeft,
  tablerPencil,
  tablerTrash,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../../modal/modal.component';
import FieldRendererComponent from '../field-renderers/field-renderer.component';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { ToastService } from '../../toast.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, KubeResource, CrdPropertySchema, PluginMenuItem } from '../types';
import {
  formatDate,
  fieldNameToLabel,
  groupFields,
  resolveStatusBadge,
  kindToSingularLabel,
} from '../crd-schema.utils';

function buildListLink(): string[] {
  return ['..'];
}

function buildEditLink(): string[] {
  return ['edit'];
}

function toDateValue(val: unknown): string {
  return formatDate(String(val ?? ''));
}

function toSimpleValue(val: unknown): string {
  if (val === null || val === undefined) return '\u2014';
  if (typeof val === 'object') return JSON.stringify(val);
  return String(val);
}

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
  standalone: true,
  imports: [RouterLink, NgIcon, ModalComponent, FieldRendererComponent],
  viewProviders: [
    provideIcons({ tablerAlertTriangle, tablerArrowLeft, tablerPencil, tablerTrash }),
  ],
  templateUrl: './resource-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceDetailComponent {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private toastService = inject(ToastService);

  private titleService = inject(TitleService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  private resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  crdDef = computed<ParsedCrd | undefined>(() =>
    this.registry.getCrdByPlural(this.pluginName(), this.resourceKind()),
  );

  menuItem = computed<PluginMenuItem | undefined>(() => {
    const p = this.plugin();
    const crd = this.crdDef();
    if (!p || !crd) return undefined;
    const allItems = [...(p.menu.organization ?? []), ...(p.menu.project ?? [])];
    return allItems.find((item) => item.crd === crd.kind);
  });

  resource = computed<KubeResource | undefined>(() => {
    const crd = this.crdDef();
    if (!crd) return undefined;
    return this.store.getResource(this.pluginName(), crd.kind, this.resourceId());
  });

  statusBadge = computed(() => {
    const r = this.resource();
    const p = this.plugin();
    const crd = this.crdDef();
    if (!r || !p?.uiHints || !crd) return undefined;
    return resolveStatusBadge(r, p.uiHints[crd.kind]?.statusMapping);
  });

  fieldGroups = computed(() => {
    const crd = this.crdDef();
    const p = this.plugin();
    if (!crd) return [];
    const hints = p?.uiHints?.[crd.kind];
    return groupFields(crd.specSchema, hints?.formGroups, hints?.hiddenFields);
  });

  statusFields = computed<[string, unknown][]>(() => {
    const r = this.resource();
    if (!r?.status) return [];
    return Object.entries(r.status);
  });

  singularLabel = computed(() => {
    const crd = this.crdDef();
    return crd ? kindToSingularLabel(crd.kind) : 'resource';
  });

  showDeleteModal = signal(false);

  constructor() {
    effect(() => {
      const r = this.resource();
      this.titleService.setTitle(r?.metadata.name);
    });
  }

  listLink = buildListLink;

  editLink = buildEditLink;

  formatLabel = fieldNameToLabel;

  formatDateValue = toDateValue;

  formatSimpleValue = toSimpleValue;

  isWideField = checkIsWideField;

  isWideStatusField = checkIsWideStatusField;

  isConditionsField = checkIsConditionsField;

  asArray = toArray;

  asRecord = toRecord;

  getSpecValue(fieldName: string): unknown {
    return this.resource()?.spec?.[fieldName] ?? null;
  }

  openDeleteModal(): void {
    this.showDeleteModal.set(true);
  }

  confirmDelete(): void {
    const crd = this.crdDef();
    if (!crd) return;
    this.store.deleteResource(this.pluginName(), crd.kind, this.resourceId());
    this.showDeleteModal.set(false);
    this.toastService.show(`${kindToSingularLabel(crd.kind)} deleted`, 'success');
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
