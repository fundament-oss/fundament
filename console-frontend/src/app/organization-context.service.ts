import { Injectable, signal, computed } from '@angular/core';

const STORAGE_KEY = 'selected_organization_id';

const NAME_STORAGE_KEY = 'selected_organization_name';

/**
 * Service to track the currently selected organization.
 * This is used to set the Fun-Organization header in API requests.
 *
 * The in-memory signal is the source of truth per tab. localStorage is used
 * as a shared hint so that new tabs/windows default to the last selected org,
 * while each tab can independently switch to a different org.
 *
 * Two things identify the same organization here, because two readers want
 * different ones. The API knows it by its id, a UUID, and that is what the
 * header carries. An address names it by its name, which is unique, cannot be
 * changed and reads as a word — `gemeente-fundament` rather than
 * `019a3f2b-7c41-…`. The name arrives first, straight from the address; the id
 * follows once the organization itself has been fetched.
 */
@Injectable({
  providedIn: 'root',
})
export default class OrganizationContextService {
  private readonly organizationId = signal<string | null>(null);

  private readonly organizationName = signal<string | null>(null);

  /**
   * The current organization ID. Used to set the Fun-Organization header.
   */
  readonly currentOrganizationId = this.organizationId.asReadonly();

  /** The name the address uses, which is what every link is built from. */
  readonly currentOrganizationName = this.organizationName.asReadonly();

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

  /** Set the name the address goes by, read straight from that address. */
  setOrganizationName(name: string | null) {
    this.organizationName.set(name);
    if (name) {
      localStorage.setItem(NAME_STORAGE_KEY, name);
    } else {
      localStorage.removeItem(NAME_STORAGE_KEY);
    }
  }

  /**
   * Get the stored organization ID from localStorage.
   * Used as a default when initializing a new tab.
   */
  static getStoredOrganizationId(): string | null {
    return localStorage.getItem(STORAGE_KEY);
  }

  /** The organization this browser was last in, for an address without one. */
  static getStoredOrganizationName(): string | null {
    return localStorage.getItem(NAME_STORAGE_KEY);
  }

  /**
   * Clear the organization ID from both signal and localStorage.
   */
  clearOrganizationId() {
    this.organizationId.set(null);
    this.organizationName.set(null);
    localStorage.removeItem(STORAGE_KEY);
    localStorage.removeItem(NAME_STORAGE_KEY);
  }
}
