import {
  Component,
  ViewChild,
  ElementRef,
  AfterViewInit,
  OnInit,
  inject,
  signal,
  ChangeDetectionStrategy,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import LoadingIndicatorComponent from '../icons/loading-indicator.component';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import { CreateProjectRequestSchema } from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  type ListClustersResponse_ClusterSummary as ClusterSummary,
} from '../../generated/v1/cluster_pb';

@Component({
  selector: 'app-add-project',
  imports: [RouterLink, ReactiveFormsModule, NgIcon, LoadingIndicatorComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
    }),
  ],
  templateUrl: './add-project.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class AddProjectComponent implements AfterViewInit, OnInit {
  @ViewChild('projectNameInput') projectNameInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(TitleService);

  private router = inject(Router);

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

  constructor() {
    this.titleService.setTitle('Add a project');
  }

  async ngOnInit() {
    await this.loadClusters();
  }

  ngAfterViewInit() {
    // Focus the project name input after the view is initialized
    this.projectNameInput.nativeElement.focus();
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

  async onSubmit() {
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

      const response = await firstValueFrom(this.client.createProject(request));

      this.toastService.success(`Project '${this.projectForm.value.name}' created successfully`);

      // Reload organization data to update the selector modal
      await this.organizationDataService.loadOrganizationData();

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
