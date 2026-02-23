import { Injectable, signal, computed } from '@angular/core';

const STORAGE_KEY = 'selected_organization_id';

/**
 * Service to track the currently selected organization.
 * This is used to set the Fun-Organization header in API requests.
 *
 * The in-memory signal is the source of truth per tab. localStorage is used
 * as a shared hint so that new tabs/windows default to the last selected org,
 * while each tab can independently switch to a different org.
 */
@Injectable({
  providedIn: 'root',
})
export default class OrganizationContextService {
  private readonly organizationId = signal<string | null>(null);

  /**
   * The current organization ID. Used to set the Fun-Organization header.
   */
  readonly currentOrganizationId = this.organizationId.asReadonly();

  /**
   * Whether an organization is currently selected.
   */
  readonly hasOrganization = computed(() => this.organizationId() !== null);

  /**
   * Set the current organization ID and persist to localStorage.
   */
  setOrganizationId(id: string | null) {
    this.organizationId.set(id);
    if (id) {
      localStorage.setItem(STORAGE_KEY, id);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  }

  /**
   * Get the stored organization ID from localStorage.
   * Used as a default when initializing a new tab.
   */
  static getStoredOrganizationId(): string | null {
    return localStorage.getItem(STORAGE_KEY);
  }

  /**
   * Clear the organization ID from both signal and localStorage.
   */
  clearOrganizationId() {
    this.organizationId.set(null);
    localStorage.removeItem(STORAGE_KEY);
  }
}
