import { Injectable, inject, signal } from '@angular/core';
import type { User } from '../generated/authn/v1/authn_pb';
import SessionService from './session.service';

const STORAGE_KEY = 'marketplace_selected_organization_id';

/**
 * The organization the developer portal acts for. Every registry.v1 RPC is
 * organization-scoped through the Fun-Organization header (FUN-6), and the
 * membership list comes off the session SessionService resolves — the same
 * cookie-backed session the registry validates, so the header can never name
 * an organization the token does not.
 *
 * localStorage remembers the last selection; a stored id that is no longer in
 * the membership list falls back to the first membership. A visitor with no
 * session has no organizations, which the portal's auth guard turns into a
 * login before any of this is asked for; on a build with no session surface at
 * all (the demo) the header simply stays unset.
 */
@Injectable({ providedIn: 'root' })
export default class OrganizationContextService {
  private readonly session = inject(SessionService);

  readonly organizationIds = signal<string[]>([]);

  readonly organizationId = signal<string | null>(null);

  private loaded?: Promise<string | null>;

  // The session user `loaded` was worked out from.
  private loadedFor: User | null = null;

  /**
   * Resolves the active organization id, working it out once per session.
   *
   * Every registry RPC asks for it, so it is not simply dropped when it comes
   * back empty: that would put a GetUserInfo in front of each call from a tab
   * with no session. Instead it is redone when the session changes under it,
   * which is how a login in another tab arrives: SessionService does not keep
   * an empty session, so the guard's next ensureUser picks the new one up.
   */
  ensureOrganizationId(): Promise<string | null> {
    if (this.loaded && this.session.user() !== this.loadedFor) {
      this.loaded = undefined;
    }
    this.loaded ??= this.load();
    return this.loaded;
  }

  setOrganizationId(id: string) {
    this.organizationId.set(id);
    this.loaded = Promise.resolve(id);
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, id);
    }
  }

  private async load(): Promise<string | null> {
    // SessionService swallows the unauthenticated case, so no session means no
    // memberships: the header is left unset and the registry answers with its
    // own error.
    const user = await this.session.ensureUser();
    this.loadedFor = user;
    const ids = user?.organizationIds ?? [];
    this.organizationIds.set(ids);

    const stored = typeof localStorage === 'undefined' ? null : localStorage.getItem(STORAGE_KEY);
    const active = stored && ids.includes(stored) ? stored : (ids[0] ?? null);
    this.organizationId.set(active);
    return active;
  }
}
