import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  effect,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router, RouterOutlet } from '@angular/router';
import { ReactiveFormsModule, FormBuilder } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import PageNavService from '../page-nav.service';
import { OverlayService } from '../overlay.service';
import { OrganizationDataService } from '../organization-data.service';
import {
  ListProjectNamespacesRequestSchema,
  Namespace,
} from '../../generated/v1/namespace_pb';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { formatDate as formatDateUtil } from '../utils/date-format';
import { mockBindingsFor } from '../utils/mock-role-bindings';
import { ALL_NAMESPACES } from '../utils/namespace-grants';
import { NAMESPACE, PROJECT } from '../../connect/tokens';
import type { ProjectMember } from '../../generated/v1/project_pb';

@Component({
  selector: 'app-namespaces',
  imports: [
    ReactiveFormsModule,
    DialogSyncDirective,
    SheetSyncDirective,
    AutofocusDirective,
    RouterOutlet,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './namespaces.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NamespacesComponent implements OnInit {
  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  protected overlays = inject(OverlayService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private fb = inject(FormBuilder);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');

  /** Named in the sheet title, so it says which project the namespace lands in;
   *  the page header is behind the sheet while it is open. */
  projectName = computed(() => {
    const gevonden = this.organizationDataService.getProjectById(this.projectId())?.project;
    return gevonden?.alias || gevonden?.name || 'this project';
  });

  namespaces = signal<Namespace[]>([]);

  /** True until the first load lands: an empty list and a list not yet loaded
   *  look the same, and only one of them should offer to create something. */
  isLoading = signal(true);

  members = signal<ProjectMember[]>([]);

  namespaceNames = computed(() => this.namespaces().map((namespace) => namespace.name));

  errorMessage = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Namespaces');
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    await Promise.all([this.loadNamespaces(projectId), this.loadMembers(projectId)]);
  }

  /** The sheet that creates one lives in the shell and may well be standing
   *  over this very list, so the list hears about a new namespace from there. */
  private readonly reloadOnCreate = effect(() => {
    this.organizationDataService.namespacesChanged();
    const projectId = this.projectId();
    if (projectId) this.loadNamespaces(projectId);
  });

  /** Only for the member summary on a row; the sheet has the detail. */
  async loadMembers(projectId: string) {
    try {
      const response = await firstValueFrom(this.projectClient.listProjectMembers({ projectId }));
      this.members.set(response.members);
    } catch {
      this.members.set([]);
    }
  }

  /** Who works in this namespace. One of them is worth naming: the name says
   *  more than the number does. */
  memberSummary = (namespace: string): string => {
    const names = this.members()
      .filter((member) => {
        const bindings = mockBindingsFor(member.id, this.namespaceNames());
        return bindings.some(
          (binding) => binding.namespace === namespace || binding.namespace === ALL_NAMESPACES,
        );
      })
      .map((member) => member.userName);
    if (names.length === 1) return names[0];
    return `${names.length} members`;
  };

  /** Routes client-side while leaving the row a real link, so middle-click and
   *  "open in new tab" keep working. */
  openNamespace(event: Event, name: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }
    event.preventDefault();
    this.router.navigateByUrl(`/projects/${this.projectId()}/namespaces/${name}`);
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      this.notificationService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  readonly formatDate = formatDateUtil;

}
