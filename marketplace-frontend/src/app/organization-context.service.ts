import { Injectable, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { AUTHN_CLIENT } from '../connect/authn';

const STORAGE_KEY = 'marketplace_selected_organization_id';

/**
 * The organization the developer portal acts for. Every registry.v1 RPC is
 * organization-scoped through the Fun-Organization header (FUN-6), and the
 * membership list comes from authn.v1 GetUserInfo — the same cookie-backed
 * session the registry validates, so the header can never name an
 * organization the token does not.
 *
 * localStorage remembers the last selection; a stored id that is no longer in
 * the membership list falls back to the first membership. Developer login via
 * the console is deferred (FUN-20), so an unauthenticated visitor simply has
 * no organizations and the portal's calls fail with the API's own error.
 */
@Injectable({ providedIn: 'root' })
export default class OrganizationContextService {
  private readonly authnClient = inject(AUTHN_CLIENT);

  readonly organizationIds = signal<string[]>([]);

  readonly organizationId = signal<string | null>(null);

  private loaded?: Promise<string | null>;

  /** Resolves the active organization id, fetching the membership once. */
  ensureOrganizationId(): Promise<string | null> {
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
    try {
      const response = await firstValueFrom(this.authnClient.getUserInfo({}));
      const ids = response.user?.organizationIds ?? [];
      this.organizationIds.set(ids);

      const stored = typeof localStorage === 'undefined' ? null : localStorage.getItem(STORAGE_KEY);
      const active = stored && ids.includes(stored) ? stored : (ids[0] ?? null);
      this.organizationId.set(active);
      return active;
    } catch {
      // Not signed in (or authn unreachable): leave the header unset and let
      // the registry answer with its own error.
      this.organizationIds.set([]);
      this.organizationId.set(null);
      return null;
    }
  }
}
