import {
  Component,
  inject,
  signal,
  computed,
  effect,
  untracked,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { Code, ConnectError } from '@connectrpc/connect';
import { NAMESPACE } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { formatDate } from '../utils/date-format';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/container';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

type LoadState = 'loading' | 'found' | 'missing' | 'failed';

/**
 * Everything about one namespace. Who may work in it is not decided here: access
 * is granted on the project and every namespace in it follows.
 */
@Component({
  selector: 'app-namespace-sheet',
  imports: [DialogSyncDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './namespace-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NamespaceSheetComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private namespaceClient = inject(NAMESPACE);

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  projectId = signal('');

  namespaceName = signal('');

  /** Set once the namespace is found; the delete call needs it. */
  namespaceId = signal('');

  created = signal('');

  /** How the call for the namespace ended. "missing" is the server saying it
   *  does not exist; "failed" is anything else, which says nothing about the
   *  namespace and can be tried again. */
  private fetchState = signal<LoadState>('loading');

  /** The namespace is asked for by cluster, project and namespace name, and the
   *  first two come with the organization data. Compared by value, so a reload
   *  that brings back the same names is not a reason to ask again. */
  private target = computed(
    () => {
      const entry = this.organizationData.getProjectById(this.projectId());
      return entry ? { clusterName: entry.cluster.name, projectName: entry.project.name } : null;
    },
    {
      equal: (a, b) => a?.clusterName === b?.clusterName && a?.projectName === b?.projectName,
    },
  );

  /** What the sheet shows. Without the project there is nothing to ask for
   *  yet: that is loading until the projects are in, failed when their list
   *  did not come back, and only then a namespace that cannot exist. */
  state = computed<LoadState>(() => {
    if (this.target()) return this.fetchState();
    if (this.organizationData.organizationLoadFailed()) return 'failed';
    if (!this.organizationData.projectsSettled()) return 'loading';
    return this.organizationData.projectLoadFailures().size > 0 ? 'failed' : 'missing';
  });

  retrying = signal(false);

  showDeleteModal = signal(false);

  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  ngOnInit() {
    // The project id lives on the parent route: this sheet is its child.
    this.projectId.set(this.route.snapshot.parent?.params['id'] ?? '');
    this.namespaceName.set(this.route.snapshot.params['name'] ?? '');
  }

  /** Asks once the project is known, which on a direct link is after the
   *  sheet opened. Not again once the namespace is in: deleting it reloads the
   *  projects, and asking for it then would say "not found" on the way out. */
  private readonly loadOnTarget = effect(() => {
    if (!this.target()) return;
    untracked(() => {
      if (this.fetchState() !== 'found') this.load();
    });
  });

  async load() {
    const target = this.target();
    if (!target) return;

    this.fetchState.set('loading');
    try {
      const response = await firstValueFrom(
        this.namespaceClient.getNamespaceByProjectAndName({
          ...target,
          namespaceName: this.namespaceName(),
        }),
      );
      this.namespaceId.set(response.namespace?.id ?? '');
      this.created.set(
        response.namespace?.created ? formatDate(response.namespace.created) : 'unknown',
      );
      this.fetchState.set(response.namespace ? 'found' : 'missing');
    } catch (err) {
      this.fetchState.set(
        err instanceof ConnectError && err.code === Code.NotFound ? 'missing' : 'failed',
      );
    }
  }

  /** The button reports the retry: the failure it answers may come straight
   *  back, and the sheet would otherwise look untouched. */
  async retry() {
    this.retrying.set(true);
    try {
      if (this.target()) {
        await this.load();
        return;
      }
      // The project is what is missing, and it hangs off the organization.
      if (this.organizationData.organizationLoadFailed()) {
        await this.organizationData.loadOrganizationData();
        if (this.organizationData.organizationLoadFailed()) return;
      }
      await this.organizationData.reloadProjectsAndNamespaces();
    } catch {
      // The sheet shows the failure again; nothing more to say here.
    } finally {
      this.retrying.set(false);
    }
  }

  async deleteNamespace() {
    this.showDeleteModal.set(false);
    try {
      await firstValueFrom(
        this.namespaceClient.deleteNamespace({ namespaceId: this.namespaceId() }),
      );
      this.notificationService.success(`Namespace '${this.namespaceName()}' deleted`);
      // The list this sheet opened over is a parent route, so it is still
      // mounted and would go on showing the row that was just deleted. Same
      // signal the create sheet bumps, and bumped before the sheet closes so
      // the row is gone by the time the list is uncovered.
      this.organizationData.namespacesChanged.update((count) => count + 1);
      // The namespace count rides along on the project, so it is the projects
      // that have to come back, not the organization: reloading that one drops
      // every project it holds and nothing fetches them again.
      //
      // Swallowed, and after the notification: the namespace is gone either way,
      // and a failed refresh reported as "Namespace not deleted" would say the
      // opposite of what happened. The list it could not refresh keeps what it
      // had, and the next project that opens fetches it again.
      await this.organizationData.reloadProjectsAndNamespaces().catch(() => {});
      this.onClose();
    } catch (err) {
      this.notificationService.error(
        err instanceof Error ? `Namespace not deleted: ${err.message}` : 'Namespace not deleted',
      );
    }
  }

  /** Back to the list this sheet was opened from. */
  onClose(): void {
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
