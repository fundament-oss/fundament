import {
  Component,
  OnInit,
  Input,
  Output,
  EventEmitter,
  inject,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import { CreateProjectRequestSchema } from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';
import AutofocusDirective from '../autofocus.directive';
import SheetSyncDirective from '../sheet-sync.directive';

@Component({
  selector: 'app-add-project',
  imports: [ReactiveFormsModule, AutofocusDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './add-project.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class AddProjectComponent implements OnInit {
  /** Owned by the shell: the sheet opens over the page you were on, and that
   *  page stays mounted behind it. */
  @Input() show = false;

  @Output() closed = new EventEmitter<void>();

  private router = inject(Router);

  private idempotency = createIdempotencyRef();

  private fb = inject(FormBuilder);

  private client = inject(PROJECT);

  private clusterClient = inject(CLUSTER);

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

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
        Validators.maxLength(63),
        Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
      ],
    ],
  });

  async ngOnInit() {
    await this.loadClusters();
  }

  async loadClusters() {
    try {
      this.isLoadingClusters.set(true);
      const request = create(ListClustersRequestSchema, {});
      const response = await firstValueFrom(this.clusterClient.listClusters(request));
      this.clusters.set(response.clusters);
      if (response.clusters.length > 0) {
        this.projectForm.patchValue({ clusterId: response.clusters[0].id });
      }
    } catch (error) {
      this.toastService.error(
        error instanceof Error
          ? `Failed to load clusters: ${error.message}`
          : 'Failed to load clusters',
      );
    } finally {
      this.isLoadingClusters.set(false);
    }
  }

  onClusterChange(event: Event): void {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.projectForm.get('clusterId')?.setValue(value);
    this.projectForm.get('clusterId')?.markAsDirty();
  }

  /** Dismissing the sheet leaves the flow, which unmounts this route and reveals
   *  the project list the sheet was covering. */
  onCancel(): void {
    this.closed.emit();
  }

  async onSubmit(event?: Event) {
    event?.preventDefault();

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

      this.toastService.success(`Project '${this.projectForm.value.name}' created successfully`);

      // Reload project data to update the selector modal and breadcrumbs
      await this.organizationDataService.reloadProjectsAndNamespaces();

      this.closed.emit();
      this.router.navigate(['/projects', response.projectId]);
    } catch (error) {
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
      return 'Project name must not exceed 63 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return 'Project name must contain only lowercase letters, numbers, and hyphens, start with a letter, and end with a letter or number.';
    }
    return '';
  }
}
