import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  OnInit,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import FormFieldComponent from '../field-renderers/form-field.component';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { ToastService } from '../../toast.service';
import type { ParsedCrd, KubeResource } from '../types';
import { groupFields, buildDefaultValue, isFieldRequired } from '../crd-schema.utils';

function buildListLink(): string[] {
  return ['..'];
}

@Component({
  selector: 'app-resource-create',
  standalone: true,
  imports: [FormsModule, RouterLink, FormFieldComponent],
  templateUrl: './resource-create.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceCreateComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private toastService = inject(ToastService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  crdDef = computed<ParsedCrd | undefined>(() =>
    this.registry.getCrdByPlural(this.pluginName(), this.resourceKind()),
  );

  fieldGroups = computed(() => {
    const crd = this.crdDef();
    const p = this.plugin();
    if (!crd) return [];
    const hints = p?.uiHints?.[crd.kind];
    return groupFields(crd.specSchema, hints?.formGroups, hints?.hiddenFields);
  });

  resourceName = '';

  resourceNamespace = 'default';

  private formData = signal<Record<string, unknown>>({});

  listLink = buildListLink;

  ngOnInit(): void {
    const crd = this.crdDef();
    if (!crd) return;

    const defaults: Record<string, unknown> = {};
    Object.entries(crd.specSchema.properties).forEach(([name, schema]) => {
      defaults[name] = buildDefaultValue(schema);
    });
    this.formData.set(defaults);
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
    if (!crd) return;

    if (!this.resourceName.trim()) {
      this.toastService.show('Name is required', 'error');
      return;
    }

    if (crd.scope === 'Namespaced' && !this.resourceNamespace.trim()) {
      this.toastService.show('Namespace is required', 'error');
      return;
    }

    // Check required spec fields
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

    const resource: KubeResource = {
      apiVersion: `${crd.group}/${crd.version}`,
      kind: crd.kind,
      metadata: {
        name: this.resourceName.trim(),
        namespace: crd.scope === 'Namespaced' ? this.resourceNamespace.trim() : undefined,
        uid: '',
        creationTimestamp: '',
      },
      spec,
    };

    this.store.createResource(this.pluginName(), crd.kind, resource);
    this.toastService.show(`${crd.singular} created`, 'success');
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
