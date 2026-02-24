import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
  OnInit,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import FormFieldComponent from '../field-renderers/form-field.component';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { ToastService } from '../../toast.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, KubeResource } from '../types';
import { groupFields, isFieldRequired, kindToSingularLabel } from '../crd-schema.utils';

function buildDetailLink(): string[] {
  return ['..'];
}

@Component({
  selector: 'app-resource-edit',
  standalone: true,
  imports: [FormsModule, RouterLink, FormFieldComponent],
  templateUrl: './resource-edit.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceEditComponent implements OnInit {
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

  resource = computed<KubeResource | undefined>(() => {
    const crd = this.crdDef();
    if (!crd) return undefined;
    return this.store.getResource(this.pluginName(), crd.kind, this.resourceId());
  });

  fieldGroups = computed(() => {
    const crd = this.crdDef();
    const p = this.plugin();
    if (!crd) return [];
    const hints = p?.uiHints?.[crd.kind];
    if (hints?.editableFields) {
      const allFields = Object.keys(crd.specSchema.properties);
      const hiddenFields = allFields.filter((f) => !hints.editableFields!.includes(f));
      return groupFields(crd.specSchema, hints.formGroups, hiddenFields);
    }
    return groupFields(crd.specSchema, hints?.formGroups, hints?.hiddenFields);
  });

  singularLabel = computed(() => {
    const crd = this.crdDef();
    return crd ? kindToSingularLabel(crd.kind) : 'resource';
  });

  private formData = signal<Record<string, unknown>>({});

  detailLink = buildDetailLink;

  constructor() {
    effect(() => {
      const r = this.resource();
      this.titleService.setTitle(`Edit ${r?.metadata.name ?? this.singularLabel()}`);
    });
  }

  ngOnInit(): void {
    const resource = this.resource();
    if (resource) {
      this.formData.set(structuredClone(resource.spec) as Record<string, unknown>);
    }
  }

  getFormValue(fieldName: string): unknown {
    return this.formData()[fieldName] ?? null;
  }

  setFormValue(fieldName: string, value: unknown): void {
    this.formData.update((current) => ({ ...current, [fieldName]: value }));
  }

  isRequired(fieldName: string): boolean {
    const crd = this.crdDef();
    if (!crd) return false;
    return isFieldRequired(fieldName, crd.specSchema);
  }

  onSubmit(): void {
    const crd = this.crdDef();
    const existing = this.resource();
    if (!crd || !existing) return;

    const missingField = (crd.specSchema.required ?? []).find((reqField) => {
      const val = this.formData()[reqField];
      return val === null || val === undefined || val === '';
    });
    if (missingField) {
      this.toastService.show(`${missingField} is required`, 'error');
      return;
    }

    // Clean spec data: remove empty strings, null values, empty arrays
    const spec: Record<string, unknown> = {};
    Object.entries(this.formData()).forEach(([key, val]) => {
      if (val === '' || val === null) return;
      if (Array.isArray(val) && val.length === 0) return;
      if (typeof val === 'object' && val !== null && !Array.isArray(val)) {
        const obj = val as Record<string, unknown>;
        if (Object.keys(obj).length === 0) {
          spec[key] = obj;
          return;
        }
        const cleaned: Record<string, unknown> = {};
        let hasValue = false;
        Object.entries(obj).forEach(([k, v]) => {
          if (v !== '' && v !== null) {
            cleaned[k] = v;
            hasValue = true;
          }
        });
        if (hasValue) spec[key] = cleaned;
        return;
      }
      spec[key] = val;
    });

    const updated: KubeResource = { ...existing, spec };
    this.store.updateResource(this.pluginName(), crd.kind, this.resourceId(), updated);
    this.toastService.show(`${kindToSingularLabel(crd.kind)} updated`, 'success');
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
