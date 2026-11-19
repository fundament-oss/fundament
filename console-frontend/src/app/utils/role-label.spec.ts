import { ProjectMemberRole } from '../../generated/v1/project_pb';
import { projectRoleAccess } from './role-label';

describe('projectRoleAccess', () => {
  it('names the role a project admin is bound to in every namespace', () => {
    expect(projectRoleAccess(ProjectMemberRole.ADMIN, false)).toBe(
      'admin in every namespace of this project',
    );
  });

  it('names the role a project viewer is bound to in every namespace', () => {
    expect(projectRoleAccess(ProjectMemberRole.VIEWER, false)).toBe(
      'view in every namespace of this project',
    );
  });

  it('gives an organization admin cluster-admin, whatever the project role', () => {
    expect(projectRoleAccess(ProjectMemberRole.ADMIN, true)).toBe(
      'cluster-admin on the whole cluster',
    );
  });

  it('says nothing for a role it does not know', () => {
    expect(projectRoleAccess(ProjectMemberRole.UNSPECIFIED, false)).toBeNull();
  });
});
