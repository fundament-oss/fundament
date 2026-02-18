import { Injectable, signal, computed } from '@angular/core';

/**
 * Service to track the currently selected organization.
 * This is used to set the Fun-Organization header in API requests.
 */
@Injectable({
  providedIn: 'root',
})
export class OrganizationContextService {
  private readonly _currentOrganizationId = signal<string | null>(null);

  /**
   * The current organization ID. Used to set the Fun-Organization header.
   */
  readonly currentOrganizationId = this._currentOrganizationId.asReadonly();

  /**
   * Whether an organization is currently selected.
   */
  readonly hasOrganization = computed(() => this._currentOrganizationId() !== null);

  /**
   * Set the current organization ID.
   */
  setOrganizationId(organizationId: string | null) {
    this._currentOrganizationId.set(organizationId);
  }
}
