import {
  Component,
  Output,
  EventEmitter,
  inject,
  input,
  signal,
  computed,
  effect,
  untracked,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router } from '@angular/router';
import { AbstractControl, ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { ConnectError, Code } from '@connectrpc/connect';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import { CreateProjectRequestSchema } from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import AutofocusDirective from '../autofocus.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import PageNavService from '../page-nav.service';
import NavContextService from '../nav-context.service';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/link';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/toggle-button';
import '@nldd/design-system/toggle-button-group';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';

@Component({
  selector: 'app-new-project',
  imports: [ReactiveFormsModule, AutofocusDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './new-project.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NewProjectComponent {
  protected pageNav = inject(PageNavService);

  /** Owned by the shell: the sheet opens over the page you were on, and that
   *  page stays mounted behind it. */
  show = input(false);

  private navContext: NavContextService = inject(NavContextService);

  /** Opened inside a cluster or a project: the project goes on that cluster,
   *  and there is nothing to choose. The sheet covers the page without
   *  changing the address, so the place is the one it was opened in. Opened
   *  from the organization (or by its own address), the user picks. */
  protected scoped = computed(() => this.navContext.context().kind !== 'organization');

  /** The cluster a scoped project goes on, from the organization's data, so
   *  it does not depend on the list this sheet fetched. Null while a project's
   *  cluster is still being looked up. */
  protected fixedCluster = computed(() => {
    if (!this.scoped()) return null;
    const id = this.navContext.clusterId();
    return id
      ? (this.organizationDataService.clusterSummaries().find((cluster) => cluster.id === id) ??
          null)
      : null;
  });

  /** A scoped sheet whose cluster cannot be found: the projects have loaded
   *  and the place is still not among them, or the organization itself could
   *  not be loaded and never will be without a reload. Offering a choice
   *  instead would put the project somewhere the user did not start from. */
  protected fixedClusterMissing = computed(
    () =>
      this.scoped() &&
      !this.fixedCluster() &&
      (this.organizationDataService.organizationLoadFailed() ||
        (this.organizationDataService.clustersLoaded() &&
          (this.navContext.context().kind === 'cluster' ||
            this.organizationDataService.projectsSettled()))),
  );

  @Output() closed = new EventEmitter<void>();

  private router = inject(Router);

  private idempotency = createIdempotencyRef();

  private fb = inject(FormBuilder);

  private client = inject(PROJECT);

  private clusterClient = inject(CLUSTER);

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  /** Whether the form has been submitted at least once. Before that, a field
   * that is not finished yet is not a mistake, only unfinished. */
  formSubmitted = signal(false);

  errorMessage = signal<string | null>(null);

  isSubmitting = signal<boolean>(false);

  clusters = signal<ClusterSummary[]>([]);

  isLoadingClusters = signal<boolean>(false);

  projectForm = this.fb.group({
    clusterId: ['', Validators.required],
    name: [
      '',
      [
        Validators.required,
        Validators.minLength(1),
        Validators.maxLength(30),
        Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
        // "--" separates the project from the namespace in cluster-side names.
        (control: AbstractControl<string | null>) =>
          control.value?.includes('--') ? { consecutiveHyphens: true } : null,
      ],
    ],
  });

  constructor() {
    // Each opening starts over: the component outlives the sheet (the shell
    // defers it once and keeps it), so the cluster list is fetched again, as a
    // cluster may have been added since, and the choice made last time does
    // not carry over.
    effect(() => {
      if (!this.show()) return;
      untracked(() => {
        this.projectForm.patchValue({ clusterId: this.fixedCluster()?.id ?? '' });
        this.loadClusters();
      });
    });

    // A scoped sheet's cluster is the place's, kept in the form so submitting
    // sends it. Empty until it is known, which the form's `required` refuses.
    effect(() => {
      if (!this.scoped()) return;
      const id = this.fixedCluster()?.id ?? '';
      untracked(() => this.projectForm.patchValue({ clusterId: id }));
    });
  }

  // Reopening can overlap a fetch; only the newest one applies.
  private clusterLoad = 0;

  async loadClusters() {
    this.clusterLoad += 1;
    const current = this.clusterLoad;
    try {
      this.isLoadingClusters.set(true);
      const request = create(ListClustersRequestSchema, {});
      const response = await firstValueFrom(this.clusterClient.listClusters(request));
      if (current !== this.clusterLoad) return;
      this.clusters.set(response.clusters);
      if (!this.scoped() && !this.projectForm.value.clusterId && response.clusters.length > 0) {
        this.projectForm.patchValue({ clusterId: response.clusters[0].id });
      }
    } catch (error) {
      if (current !== this.clusterLoad) return;
      this.notificationService.error(
        error instanceof Error
          ? `Failed to load clusters: ${error.message}`
          : 'Failed to load clusters',
      );
    } finally {
      if (current === this.clusterLoad) this.isLoadingClusters.set(false);
    }
  }

  onClusterChange(event: Event): void {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.projectForm.get('clusterId')?.setValue(value);
    this.projectForm.get('clusterId')?.markAsDirty();
    // A name taken on one cluster can be free on another.
    this.projectForm.get('name')?.updateValueAndValidity();
  }

  /** Dismissing the sheet leaves the flow, which unmounts this route and reveals
   *  the project list the sheet was covering. */
  onCancel(): void {
    this.closed.emit();
  }

  async onSubmit(event?: Event) {
    event?.preventDefault();
    // A second submit would start a create under a new idempotency key.
    if (this.isSubmitting()) return;
    this.formSubmitted.set(true);

    if (this.projectForm.invalid) {
      this.projectForm.markAllAsTouched();
      return;
    }

    try {
      this.isSubmitting.set(true);
      this.errorMessage.set(null);

      const request = create(CreateProjectRequestSchema, {
        clusterId: this.projectForm.value.clusterId!,
        name: this.projectForm.value.name!,
      });

      const response = await withIdempotency((opts) => this.client.createProject(request, opts), {
        signal: this.idempotency.reset(),
      });

      this.notificationService.success(
        `Project '${this.projectForm.value.name}' created successfully`,
      );

      // Reload project data to update the sidebar and breadcrumbs. The project
      // exists by now, so a failed reload is not a failed create.
      await this.organizationDataService.reloadProjectsAndNamespaces().catch(() => {});

      this.closed.emit();
      this.pageNav.goTo(`/projects/${response.projectId}`);
    } catch (error) {
      if (error instanceof ConnectError && error.code === Code.AlreadyExists) {
        this.projectForm.get('name')?.setErrors({ taken: true });
        return;
      }
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to create project: ${error.message}`
          : 'Failed to create project',
      );
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.projectForm.get('name')?.setValue(value);
    this.projectForm.get('name')?.markAsDirty();
  }

  getClusterError(): string {
    const clusterControl = this.projectForm.get('clusterId');
    if (clusterControl?.hasError('required')) {
      return 'Please select a cluster.';
    }
    return '';
  }

  getNameError(): string {
    const nameControl = this.projectForm.get('name');
    if (nameControl?.hasError('required')) {
      return 'Project name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return 'Project name must not exceed 30 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return 'Project name must contain only lowercase letters, numbers, and hyphens, start with a letter, and end with a letter or number.';
    }
    if (nameControl?.hasError('consecutiveHyphens')) {
      return 'Project name must not contain two hyphens in a row.';
    }
    if (nameControl?.hasError('taken')) {
      return 'A project with this name already exists in this cluster.';
    }
    return '';
  }
}
