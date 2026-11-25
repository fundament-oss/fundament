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
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { PROJECT } from '../../connect/tokens';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { formatTimeAgo } from '../utils/date-format';
import { projectRoleLabel } from '../utils/role-label';
import type { ProjectMember } from '../../generated/v1/project_pb';
import { ProjectMemberRole } from '../../generated/v1/project_pb';
import PageNavService from '../page-nav.service';
import opensElsewhere from '../opens-elsewhere';
import copyWithNotice from '../utils/copy-with-notice';
import emailsByUserId from '../utils/emails-by-user-id';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/list-item-segment';
import '@nldd/design-system/menu';
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

interface ProjectMemberView {
  member: ProjectMember;
  /** 'org' when the project role equals the organization role. */
  source: 'org' | 'project';
  /** From the organization member list, which this sheet loads anyway. */
  email?: string;
}

/** Locked: an organization admin is admin here too, and this page cannot change
 *  what the organization decided. */
const isLocked = (view: ProjectMemberView): boolean =>
  view.source === 'org' && view.member.role === ProjectMemberRole.ADMIN;

const formatMemberDate = (member: ProjectMember): string =>
  member.created ? formatTimeAgo(timestampDate(member.created)) : 'unknown';

/**
 * Everything one member has in this project, as a route of its own so a row can
 * be a real link: shareable, openable in a new tab, closed by the back button.
 */
@Component({
  selector: 'app-project-member-sheet',
  imports: [DialogSyncDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './project-member-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectMemberSheetComponent implements OnInit {
  protected pageNav = inject(PageNavService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  projectId = signal('');

  memberId = signal('');

  member = signal<ProjectMemberView | null>(null);

  /** The dialog is in the DOM before the member is loaded, so this has to read
   *  as a sentence with the blank still open. It said "Remove undefined". */
  removeMemberTitle = computed(() => {
    const name = this.member()?.member?.userName;
    return name ? `Remove ${name} from this project?` : 'Remove this member from this project?';
  });

  isLoading = signal(true);

  saving = signal(false);

  showRemoveModal = signal(false);

  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || this.projectId();
  });

  ProjectMemberRole = ProjectMemberRole;

  roleLabel = projectRoleLabel;

  isLocked = isLocked;

  formatMemberDate = formatMemberDate;

  ngOnInit() {
    // The parameters live on the parent route: this sheet is its child.
    this.projectId.set(this.route.snapshot.parent?.params['id'] ?? '');
    this.memberId.set(this.route.snapshot.params['memberId'] ?? '');
    this.load();
  }

  async load() {
    this.isLoading.set(true);
    try {
      const [projectResponse, orgMembers] = await Promise.all([
        firstValueFrom(this.projectClient.listProjectMembers({ projectId: this.projectId() })),
        // Through the shared cache, so the sidebar's count is of this same list.
        this.organizationData.reloadMembers(),
      ]);

      const orgRoleByUserId = new Map(
        orgMembers.filter((m) => m.externalRef).map((m) => [m.userId, m.permission]),
      );
      const found = projectResponse.members.find((m) => m.id === this.memberId());
      this.member.set(
        found
          ? {
              member: found,
              email: emailsByUserId(orgMembers).get(found.userId),
              source:
                // The organization permission is a bare string ("admin" /
                // "viewer"), so this compares values, not the labels the tags
                // show — those name their scope and would never match.
                (found.role === ProjectMemberRole.ADMIN ? 'admin' : 'viewer') ===
                orgRoleByUserId.get(found.userId)
                  ? 'org'
                  : 'project',
            }
          : null,
      );
    } catch {
      this.member.set(null);
    } finally {
      this.isLoading.set(false);
    }
  }

  async togglePermission(view: ProjectMemberView) {
    const role =
      view.member.role === ProjectMemberRole.ADMIN
        ? ProjectMemberRole.VIEWER
        : ProjectMemberRole.ADMIN;

    this.saving.set(true);
    try {
      await firstValueFrom(
        this.projectClient.updateProjectMemberRole({ memberId: view.member.id, role }),
      );
      this.notificationService.success(
        `${view.member.userName} is now ${projectRoleLabel(role).toLowerCase()}`,
      );
      await this.load();
    } catch (err) {
      this.notificationService.error(
        err instanceof Error ? `Permission not changed: ${err.message}` : 'Permission not changed',
      );
    } finally {
      this.saving.set(false);
    }
  }

  async confirmRemove() {
    const view = this.member();
    if (!view) return;

    try {
      await firstValueFrom(this.projectClient.removeProjectMember({ memberId: view.member.id }));
      this.notificationService.success(`${view.member.userName} removed from the project`);
      this.showRemoveModal.set(false);
      this.onClose();
    } catch (err) {
      this.showRemoveModal.set(false);
      this.notificationService.error(
        err instanceof Error ? `Member not removed: ${err.message}` : 'Member not removed',
      );
    }
  }

  copyUserId(view: ProjectMemberView): Promise<void> {
    return copyWithNotice(this.notificationService, view.member.userId, 'User ID');
  }

  copyEmail(view: ProjectMemberView): Promise<void> {
    if (!view.email) return Promise.resolve();
    return copyWithNotice(this.notificationService, view.email, 'Email address');
  }

  openOrganizationMembers(event: Event): void {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.pageNav.goTo('/members');
  }

  /** Back to the list, which reloads and picks up whatever changed here. */
  onClose(): void {
    this.router.navigate(['..'], { relativeTo: this.route });
  }
}
