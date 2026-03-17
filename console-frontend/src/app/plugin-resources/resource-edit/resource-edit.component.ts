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
import { groupFields, isFieldRequired, kindToSingularLabel } from '../crd-schema.utils';

function buildDetailLink(): string[] {
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
  selector: 'app-resource-edit',
  imports: [FormsModule, RouterLink, FormFieldComponent, NgIcon],
  viewProviders: [provideIcons({ tablerArrowLeft })],
  templateUrl: './resource-edit.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceEditComponent implements OnInit {
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

  private resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  resource = signal<KubeResource | undefined>(undefined);

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

  saving = signal(false);

  private formData = signal<Record<string, unknown>>({});

  detailLink = buildDetailLink;

  constructor() {
    effect(() => {
      const r = this.resource();
      this.titleService.setTitle(`Edit ${r?.metadata.name ?? this.singularLabel()}`);
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.clusterContext.loadClusters();
    } catch {
      this.errorMessage.set('Failed to load clusters.');
      return;
    }
    const clusterId = this.clusterContext.selectedClusterId();
    if (clusterId) {
      await this.loadCrdAndResource(clusterId);
    }
  }

  private async loadCrdAndResource(clusterId: string): Promise<void> {
    const orgId = this.orgContext.currentOrganizationId();
    if (!orgId) return;

    const orgApiUrl = this.configService.getConfig().organizationApiUrl;
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      await this.registry.loadCrdsForPlugin(this.pluginName(), clusterId, orgApiUrl, orgId);
      const crd =
        this.registry.getCrd(this.pluginName(), this.resourceKind()) ??
        this.registry.getCrdByPlural(this.pluginName(), this.resourceKind());
      this.crdDef.set(crd);

      if (crd) {
        await this.store.loadResources(this.pluginName(), crd, clusterId, orgApiUrl, orgId);
        const resource = this.store.getResource(
          this.pluginName(),
          crd.kind,
          this.resourceId(),
          clusterId,
        );
        this.resource.set(resource);
        if (resource) {
          this.formData.set(structuredClone(resource.spec) as Record<string, unknown>);
        }
      }
    } catch (err) {
      this.errorMessage.set(`Failed to load resource: ${err}`);
    } finally {
      this.isLoading.set(false);
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

  async onSubmit(): Promise<void> {
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

    const spec = cleanSpec(this.formData());
    const orgId = this.orgContext.currentOrganizationId();
    const orgApiUrl = this.configService.getConfig().organizationApiUrl;
    const clusterId = this.clusterContext.selectedClusterId();

    if (!orgId || !clusterId) {
      this.toastService.show('No cluster or organization selected', 'error');
      return;
    }

    this.saving.set(true);
    try {
      await this.store.patchResource(
        this.pluginName(),
        crd,
        existing.metadata.name,
        existing.metadata.namespace,
        spec,
        clusterId,
        orgApiUrl,
        orgId,
      );
      this.toastService.show(`${kindToSingularLabel(crd.kind)} updated`, 'success');
      await this.router.navigate(['..'], { relativeTo: this.route });
    } catch (err) {
      this.toastService.show(`Failed to save: ${err}`, 'error');
    } finally {
      this.saving.set(false);
    }
  }
}
