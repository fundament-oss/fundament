import { signal } from '@angular/core';
import type ProjectNamespacesService from './project-namespaces.service';

/**
 * A page's copy of its project's namespaces, for the plugin iframe it mounts.
 * The iframe gets them once, in fundament:init, and never again, so a page
 * holds it back until `settled`. Each load clears what an earlier project left,
 * and a response for a project the page has since moved away from is dropped.
 */
export default class ProjectNamespacesLoader {
  /** Names on the cluster, which the plugin passes to the Kubernetes API as is. */
  readonly names = signal<string[]>([]);

  /** The project-level name for each of names, which the plugin shows. */
  readonly displayNames = signal<Record<string, string>>({});

  /** True once the latest load has finished, failed or not. */
  readonly settled = signal(false);

  /** True when the latest load failed; plugin UIs then fall back to a free-text
   *  namespace field. */
  readonly failed = signal(false);

  private request = 0;

  constructor(
    private service: ProjectNamespacesService,
    private logTag: string,
  ) {}

  /** Loads the project's namespaces. An empty projectId (the organization-level
   *  route) has none to load and settles at once. Never throws. */
  async load(projectId: string): Promise<void> {
    this.request += 1;
    const current = this.request;
    this.settled.set(false);
    this.failed.set(false);
    this.names.set([]);
    this.displayNames.set({});
    if (!projectId) {
      this.settled.set(true);
      return;
    }
    try {
      const { names, displayNames } = await this.service.list(projectId);
      if (current !== this.request) return;
      this.names.set(names);
      this.displayNames.set(displayNames);
    } catch (err) {
      if (current !== this.request) return;
      // eslint-disable-next-line no-console
      console.error(`[${this.logTag}] Failed to load namespaces:`, err);
      this.failed.set(true);
    } finally {
      if (current === this.request) this.settled.set(true);
    }
  }
}
