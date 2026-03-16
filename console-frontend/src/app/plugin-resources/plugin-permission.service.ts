import { Injectable, inject, signal, computed } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import { MEMBER, PROJECT } from '../../connect/tokens';
import { GetMemberRequestSchema } from '../../generated/v1/member_pb';
import {
  ListProjectMembersRequestSchema,
  ProjectMemberRole,
} from '../../generated/v1/project_pb';
import AuthnApiService from '../authn-api.service';

/**
 * Resolves write permissions for plugin resources.
 *
 * - Org-scoped canWrite: user has org "admin" permission
 * - Project-scoped canWrite: user has PROJECT_MEMBER_ROLE_ADMIN in that project,
 *   OR is an org admin (org admins inherit write on all projects)
 *
 * Security note: k8s RBAC is the authoritative enforcement layer.
 * This service only drives UX affordances (show/hide create/edit/delete buttons).
 */
@Injectable({ providedIn: 'root' })
export default class PluginPermissionService {
  private memberClient = inject(MEMBER);

  private projectClient = inject(PROJECT);

  private authnService = inject(AuthnApiService);

  private orgPermission = signal<'admin' | 'viewer' | null>(null);

  // Cache keyed by projectId → admin boolean
  private projectPermissionCache = new Map<string, boolean>();

  /**
   * True when the current user is an org admin.
   * Returns false until the org permission has been loaded.
   */
  isOrgAdmin = computed(() => this.orgPermission() === 'admin');

  /**
   * Load the current user's org-level permission.
   * Safe to call multiple times — subsequent calls are no-ops if already loaded.
   */
  async loadOrgPermission(): Promise<void> {
    if (this.orgPermission() !== null) return;

    try {
      const currentUser = await this.authnService.getUserInfo();
      if (!currentUser?.id) {
        this.orgPermission.set('viewer');
        return;
      }

      const response = await firstValueFrom(
        this.memberClient.getMember(
          create(GetMemberRequestSchema, { lookup: { case: 'userId', value: currentUser.id } }),
        ),
      );
      this.orgPermission.set(
        (response.member?.permission as 'admin' | 'viewer' | undefined) ?? 'viewer',
      );
    } catch {
      this.orgPermission.set('viewer');
    }
  }

  /**
   * True when the current user can write plugin resources in the given project.
   * Org admins always return true. Otherwise checks the project membership role.
   *
   * Caches results per projectId.
   */
  async canWriteProject(projectId: string): Promise<boolean> {
    // Org admins can write everywhere
    if (this.orgPermission() === 'admin') return true;

    const cached = this.projectPermissionCache.get(projectId);
    if (cached !== undefined) return cached;

    try {
      const currentUser = await this.authnService.getUserInfo();
      if (!currentUser?.id) {
        this.projectPermissionCache.set(projectId, false);
        return false;
      }

      const response = await firstValueFrom(
        this.projectClient.listProjectMembers(
          create(ListProjectMembersRequestSchema, { projectId }),
        ),
      );

      const member = response.members.find((m) => m.userId === currentUser.id);
      const isAdmin = member?.role === ProjectMemberRole.ADMIN;
      this.projectPermissionCache.set(projectId, isAdmin);
      return isAdmin;
    } catch {
      this.projectPermissionCache.set(projectId, false);
      return false;
    }
  }
}
