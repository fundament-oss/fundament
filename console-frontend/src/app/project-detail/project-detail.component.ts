import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  viewChild,
  ElementRef,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PROJECT, CLUSTER } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetProjectRequestSchema,
  DeleteProjectRequestSchema,
  ListProjectNamespacesRequestSchema,
  Project,
  ProjectNamespace,
} from '../../generated/v1/project_pb';
import {
  ListClustersRequestSchema,
  CreateNamespaceRequestSchema,
  DeleteNamespaceRequestSchema,
  ClusterSummary,
} from '../../generated/v1/cluster_pb';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerTrash, tablerAlertTriangle, tablerPencil } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { LoadingIndicatorComponent } from '../icons';
import { ModalComponent } from '../modal/modal.component';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';

type ProjectMemberRole = 'viewer' | 'admin';

interface ProjectMember {
  id: string;
  userId: string;
  name: string;
  email: string;
  role: ProjectMemberRole;
  addedAt: string;
}

@Component({
  selector: 'app-project-detail',
  imports: [
    CommonModule,
    RouterLink,
    ReactiveFormsModule,
    NgIcon,
    LoadingIndicatorComponent,
    ModalComponent,
    BreadcrumbComponent,
  ],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerPlus,
      tablerTrash,
      tablerAlertTriangle,
      tablerPencil,
    }),
  ],
  templateUrl: './project-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProjectDetailComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private fb = inject(FormBuilder);
  private projectClient = inject(PROJECT);
  private clusterClient = inject(CLUSTER);
  private toastService = inject(ToastService);

  project = signal<Project | null>(null);
  namespaces = signal<ProjectNamespace[]>([]);
  clusters = signal<ClusterSummary[]>([]);

  activeTab = signal<'details' | 'members'>('details');

  isLoading = signal<boolean>(true);
  errorMessage = signal<string | null>(null);

  showDeleteModal = signal<boolean>(false);
  showCreateNamespaceModal = signal<boolean>(false);

  isLoadingClusters = signal<boolean>(false);
  isCreatingNamespace = signal<boolean>(false);

  // Project Members
  members = signal<ProjectMember[]>([]);
  availableUsers = signal<{ id: string; name: string; email: string }[]>([]);
  showAddMemberModal = signal<boolean>(false);
  isAddingMember = signal<boolean>(false);
  editingMember = signal<ProjectMember | null>(null);

  namespaceNameInput = viewChild<ElementRef<HTMLInputElement>>('namespaceNameInput');

  namespaceForm = this.fb.group({
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

  memberForm = this.fb.group({
    userId: ['', Validators.required],
    role: ['viewer' as ProjectMemberRole, Validators.required],
  });

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    await this.loadProject(projectId);
    this.loadMembers();
  }

  async loadProject(projectId: string) {
    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const request = create(GetProjectRequestSchema, { projectId });
      const response = await firstValueFrom(this.projectClient.getProject(request));

      if (!response.project) {
        throw new Error('Project not found');
      }

      this.project.set(response.project);
      this.titleService.setTitle(response.project.name);

      // Load namespaces and clusters
      await Promise.all([this.loadNamespaces(projectId), this.loadClusters()]);
    } catch (error) {
      console.error('Failed to fetch project:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load project: ${error.message}`
          : 'Failed to load project',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.projectClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      console.error('Failed to fetch namespaces:', error);
      this.toastService.error('Failed to load namespaces');
    }
  }

  async loadClusters() {
    try {
      this.isLoadingClusters.set(true);
      const request = create(ListClustersRequestSchema, {});
      const response = await firstValueFrom(this.clusterClient.listClusters(request));
      this.clusters.set(response.clusters);
      if (response.clusters.length > 0) {
        this.namespaceForm.patchValue({ clusterId: response.clusters[0].id });
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error);
    } finally {
      this.isLoadingClusters.set(false);
    }
  }

  getClusterName(clusterId: string): string {
    const cluster = this.clusters().find((c) => c.id === clusterId);
    return cluster?.name || clusterId;
  }

  openCreateNamespaceModal() {
    this.namespaceForm.reset();
    this.showCreateNamespaceModal.set(true);
    this.loadClusters();
    setTimeout(() => this.namespaceNameInput()?.nativeElement.focus());
  }

  async createNamespace() {
    if (this.namespaceForm.invalid || !this.project()) {
      this.namespaceForm.markAllAsTouched();
      return;
    }

    try {
      this.isCreatingNamespace.set(true);

      const request = create(CreateNamespaceRequestSchema, {
        projectId: this.project()!.id,
        clusterId: this.namespaceForm.value.clusterId!,
        name: this.namespaceForm.value.name!,
      });

      await firstValueFrom(this.clusterClient.createNamespace(request));

      this.showCreateNamespaceModal.set(false);
      this.toastService.success(`Namespace '${this.namespaceForm.value.name}' created`);
      await this.loadNamespaces(this.project()!.id);
    } catch (error) {
      console.error('Failed to create namespace:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to create namespace: ${error.message}`
          : 'Failed to create namespace',
      );
    } finally {
      this.isCreatingNamespace.set(false);
    }
  }

  async deleteNamespace(namespaceId: string, namespaceName: string) {
    if (!confirm(`Are you sure you want to delete namespace '${namespaceName}'?`)) {
      return;
    }

    try {
      const request = create(DeleteNamespaceRequestSchema, { namespaceId });
      await firstValueFrom(this.clusterClient.deleteNamespace(request));

      this.toastService.info(`Namespace '${namespaceName}' deleted`);
      await this.loadNamespaces(this.project()!.id);
    } catch (error) {
      console.error('Failed to delete namespace:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete namespace: ${error.message}`
          : 'Failed to delete namespace',
      );
    }
  }

  async deleteProject() {
    if (!this.project()) return;

    try {
      const request = create(DeleteProjectRequestSchema, {
        projectId: this.project()!.id,
      });

      await firstValueFrom(this.projectClient.deleteProject(request));

      this.showDeleteModal.set(false);
      this.toastService.info(`Project '${this.project()!.name}' deleted`);
      this.router.navigate(['/projects']);
    } catch (error) {
      console.error('Failed to delete project:', error);
      this.showDeleteModal.set(false);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete project: ${error.message}`
          : 'Failed to delete project',
      );
    }
  }

  formatDate(dateString?: string): string {
    if (!dateString) return 'Unknown';
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  }

  getNameError(): string {
    const nameControl = this.namespaceForm.get('name');
    if (nameControl?.hasError('required')) {
      return 'Namespace name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return 'Namespace name must not exceed 63 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return 'Namespace name must start with a lowercase letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens.';
    }
    return '';
  }

  // Project Members methods
  loadMembers() {
    // Mock data for project members
    this.members.set([
      {
        id: 'pm-1',
        userId: 'user-1',
        name: 'Alice Johnson',
        email: 'alice.johnson@example.com',
        role: 'admin',
        addedAt: '2024-01-15T10:30:00Z',
      },
      {
        id: 'pm-2',
        userId: 'user-2',
        name: 'Bob Smith',
        email: 'bob.smith@example.com',
        role: 'viewer',
        addedAt: '2024-02-20T14:45:00Z',
      },
      {
        id: 'pm-3',
        userId: 'user-3',
        name: 'Carol Williams',
        email: 'carol.williams@example.com',
        role: 'viewer',
        addedAt: '2024-03-10T09:15:00Z',
      },
    ]);

    // Mock available users (users not yet in the project)
    this.availableUsers.set([
      { id: 'user-4', name: 'David Brown', email: 'david.brown@example.com' },
      { id: 'user-5', name: 'Eve Davis', email: 'eve.davis@example.com' },
      { id: 'user-6', name: 'Frank Miller', email: 'frank.miller@example.com' },
    ]);
  }

  openAddMemberModal() {
    this.editingMember.set(null);
    this.memberForm.reset({ userId: '', role: 'viewer' });
    this.showAddMemberModal.set(true);
  }

  openEditMemberModal(member: ProjectMember) {
    this.editingMember.set(member);
    this.memberForm.patchValue({ userId: member.userId, role: member.role });
    this.showAddMemberModal.set(true);
  }

  saveMember() {
    if (this.memberForm.invalid) {
      this.memberForm.markAllAsTouched();
      return;
    }

    this.isAddingMember.set(true);
    const role = this.memberForm.value.role as ProjectMemberRole;

    if (this.editingMember()) {
      // Edit existing member
      const member = this.editingMember()!;
      this.members.update((members) =>
        members.map((m) => (m.id === member.id ? { ...m, role } : m)),
      );
      this.toastService.success(
        `${member.name}'s role updated to ${role === 'admin' ? 'Project admin' : 'Project member'}`,
      );
    } else {
      // Add new member
      const userId = this.memberForm.value.userId!;
      const user = this.availableUsers().find((u) => u.id === userId);

      if (user) {
        const newMember: ProjectMember = {
          id: `pm-${Date.now()}`,
          userId: user.id,
          name: user.name,
          email: user.email,
          role,
          addedAt: new Date().toISOString(),
        };

        this.members.update((members) => [...members, newMember]);
        this.availableUsers.update((users) => users.filter((u) => u.id !== userId));
        this.toastService.success(`${user.name} added to project`);
      }
    }

    this.showAddMemberModal.set(false);
    this.isAddingMember.set(false);
    this.editingMember.set(null);
  }

  removeMember(memberId: string) {
    const member = this.members().find((m) => m.id === memberId);
    if (!member) return;

    if (!confirm(`Are you sure you want to remove ${member.name} from this project?`)) {
      return;
    }

    // Move user back to available users
    this.availableUsers.update((users) => [
      ...users,
      { id: member.userId, name: member.name, email: member.email },
    ]);
    this.members.update((members) => members.filter((m) => m.id !== memberId));
    this.toastService.info(`${member.name} removed from project`);
  }

  get breadcrumbSegments(): BreadcrumbSegment[] {
    const segments: BreadcrumbSegment[] = [
      { label: 'Projects', route: '/projects' }
    ];

    if (this.project()) {
      segments.push({ label: this.project()!.name });
    }

    return segments;
  }
}
