import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
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

  isLoading = signal(true);

  /** Null while loading, and again when this namespace turns out not to exist. */
  found = signal<boolean | null>(null);

  showDeleteModal = signal(false);

  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  ngOnInit() {
    // The project id lives on the parent route: this sheet is its child.
    this.projectId.set(this.route.snapshot.parent?.params['id'] ?? '');
    this.namespaceName.set(this.route.snapshot.params['name'] ?? '');
    this.load();
  }

  async load() {
    this.isLoading.set(true);
    try {
      const response = await firstValueFrom(
        this.namespaceClient.listProjectNamespaces({ projectId: this.projectId() }),
      );
      const namespace = response.namespaces.find((entry) => entry.name === this.namespaceName());
      this.found.set(!!namespace);
      this.namespaceId.set(namespace?.id ?? '');
      this.created.set(namespace?.created ? formatDate(namespace.created) : 'unknown');
    } catch {
      this.found.set(false);
    } finally {
      this.isLoading.set(false);
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
