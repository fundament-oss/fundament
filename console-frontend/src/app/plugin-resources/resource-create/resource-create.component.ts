import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
  OnInit,
  input,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerArrowLeft } from '@ng-icons/tabler-icons';
import FormFieldComponent from '../field-renderers/form-field.component';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import KubeClusterContextService from '../kube-cluster-context.service';
import { ConfigService } from '../../config.service';
import OrganizationContextService from '../../organization-context.service';
import { ToastService } from '../../toast.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, KubeResource } from '../types';
import {
  groupFields,
  buildDefaultValue,
  isFieldRequired,
  kindToSingularLabel,
} from '../crd-schema.utils';

function buildListLink(): string[] {
  return ['..'];
}

function cleanSpec(formData: Record<string, unknown>): Record<string, unknown> {
  const spec: Record<string, unknown> = {};
  Object.entries(formData).forEach(([key, val]) => {
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
      else if (Object.keys(obj).length === 0) spec[key] = obj;
      return;
    }
    spec[key] = val;
  });
  return spec;
}

@Component({
  selector: 'app-resource-create',
  imports: [FormsModule, RouterLink, FormFieldComponent, NgIcon],
  viewProviders: [provideIcons({ tablerArrowLeft })],
  templateUrl: './resource-create.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceCreateComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private clusterContext = inject(KubeClusterContextService);

  private configService = inject(ConfigService);

  private orgContext = inject(OrganizationContextService);

  private toastService = inject(ToastService);

  private titleService = inject(TitleService);

  /** Passed by the dispatcher; guards the route at the UI level. */
  canWrite = input<boolean>(false);

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

  singularLabel = computed(() => {
    const crd = this.crdDef();
    return crd ? kindToSingularLabel(crd.kind) : 'resource';
  });

  saving = signal(false);

  private formData = signal<Record<string, unknown>>({});

  listLink = buildListLink;

  constructor() {
    effect(() => {
      this.titleService.setTitle(`Create ${this.singularLabel()}`);
    });
  }

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

  async onSubmit(): Promise<void> {
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

    const missingField = (crd.specSchema.required ?? []).find((reqField) => {
      const val = this.formData()[reqField];
      return val === null || val === undefined || val === '';
    });
    if (missingField) {
      this.toastService.show(`${missingField} is required`, 'error');
      return;
    }

    const spec = cleanSpec(this.formData());
    const namespace = crd.scope === 'Namespaced' ? this.resourceNamespace.trim() : undefined;

    const resource: Omit<KubeResource, 'metadata'> & {
      metadata: { name: string; namespace?: string };
    } = {
      apiVersion: `${crd.group}/${crd.version}`,
      kind: crd.kind,
      metadata: { name: this.resourceName.trim(), namespace },
      spec,
    };

    const orgId = this.orgContext.currentOrganizationId();
    const orgApiUrl = this.configService.getConfig().organizationApiUrl;
    const clusterId = this.clusterContext.selectedClusterId();

    if (!orgId || !clusterId) {
      this.toastService.show('No cluster or organization selected', 'error');
      return;
    }

    this.saving.set(true);
    try {
      await this.store.createResource(
        this.pluginName(),
        crd,
        namespace,
        resource,
        clusterId,
        orgApiUrl,
        orgId,
      );
      this.toastService.show(`${kindToSingularLabel(crd.kind)} created`, 'success');
      await this.router.navigate(['..'], { relativeTo: this.route });
    } catch (err) {
      this.toastService.show(`Failed to create: ${err}`, 'error');
    } finally {
      this.saving.set(false);
    }
  }
}
