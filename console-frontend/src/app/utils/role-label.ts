import { ProjectMemberRole } from '../../generated/v1/project_pb';

/**
 * What a role is called on screen.
 *
 * 'admin' reads as a value, 'Admin' as a label — and 'Admin' on its own does not
 * say admin of what. A project admin and an organization admin are different
 * standing, they appear on pages that show both, and the tag is often the only
 * thing naming which one you are looking at. So the label carries its scope.
 *
 * One place for it because these labels have to agree: the tag on a row, the
 * option list in the sheet that handed the role out, and the menu item that
 * changes it are all naming the same thing.
 */

/** The organization permission is an untyped string in the API ("viewer" or
 *  "admin", per the proto comment), so anything else is shown as it came. */
export function organizationPermissionLabel(permission: string): string {
  switch (permission) {
    case 'admin':
      return 'Organization admin';
    case 'viewer':
      return 'Organization viewer';
    default:
      return permission;
  }
}

export function projectRoleLabel(role: ProjectMemberRole): string {
  switch (role) {
    case ProjectMemberRole.ADMIN:
      return 'Project admin';
    case ProjectMemberRole.VIEWER:
      return 'Project viewer';
    default:
      return 'Unknown';
  }
}
