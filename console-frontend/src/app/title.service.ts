import { Injectable, computed, effect, inject, signal } from '@angular/core';
import { Title, Meta } from '@angular/platform-browser';
import { NavigationEnd, NavigationStart, Router } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { filter, map } from 'rxjs/operators';
import { withinOrganization } from './address';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';

/**
 * The document title, specific first and general last, with a middot between the
 * parts: someone with a screen full of tabs reads the first words.
 *
 *     Clusters · Fundament
 *     Limits · burgerzaken · Fundament
 *     Node pools · production · Clusters · Fundament
 *     Limits · burgerzaken · Gemeente Delft · Fundament
 *
 * Each step in between only joins when it tells two pages apart. A project or a
 * cluster does that: every one of them has a page called Limits or Namespaces.
 * A single organization does not, and naming it would push the page further
 * from the front of the tab.
 */
@Injectable({
  providedIn: 'root',
})
// eslint-disable-next-line import-x/prefer-default-export
export class TitleService {
  private title = inject(Title);

  private meta = inject(Meta);

  private router = inject(Router);

  private organizationData = inject(OrganizationDataService);

  private organizationContext = inject(OrganizationContextService);

  private readonly PRODUCT = 'Fundament';

  private readonly SEPARATOR = ' · ';

  /** The console itself, for the pane that shows no page of its own. */
  private readonly DEFAULT_TITLE = 'Fundament Console';

  private pageTitle = signal<string | undefined>(undefined);

  private url = toSignal(
    this.router.events.pipe(
      filter((event): event is NavigationEnd => event instanceof NavigationEnd),
      map((event) => event.urlAfterRedirects),
    ),
    { initialValue: this.router.url },
  );

  /**
   * What the page hangs under, read from the address rather than passed down:
   * every page below would otherwise have to name it again, and the one that
   * loads last would win. From near to far, so the chain keeps running from the
   * specific to the general.
   */
  private ownerNames = computed<string[]>(() => {
    // What the address says about the page, with the organization taken off:
    // that part is named separately, and only when there is another one to tell
    // it apart from.
    const url = withinOrganization(this.url());

    const projectId = url.match(/^\/projects\/([^/?#]+)/)?.[1];
    if (projectId) {
      const project = this.organizationData.getProjectById(projectId)?.project;
      return [project?.alias || project?.name].filter((name): name is string => !!name);
    }

    const clusterId = url.match(/^\/clusters\/([^/?#]+)/)?.[1];
    if (clusterId) {
      // The summaries rather than the nested organization data: that list is
      // lightweight and is the one the console loads for the current
      // organization, so it is filled long before the other.
      const summary = this.organizationData
        .clusterSummaries()
        .find((cluster) => cluster.id === clusterId);
      const name = summary?.name || this.organizationData.getClusterById(clusterId)?.cluster.name;
      return [name, 'Clusters'].filter((part): part is string => !!part);
    }

    // The catalogue a plugin came from: "Cert Manager · Plugins · Fundament".
    if (/^\/plugins\/[^/?#]+/.test(url)) return ['Plugins'];

    return [];
  });

  /** Empty until there is a second organization: see the class comment. */
  private organizationName = computed(() => {
    const organizations = this.organizationData.organizations();
    if (organizations.length < 2) return null;

    const current = this.organizationContext.currentOrganizationId();
    const organization = current ? this.organizationData.getOrganizationById(current) : undefined;
    return organization?.alias || organization?.name || null;
  });

  /** Whether a page named itself during the navigation that is running. */
  private named = false;

  constructor() {
    // A page names itself while it is being activated, so by the time the
    // navigation ends we know whether anybody did. Nobody means a route without
    // a component of its own, and then the previous name is let go rather than
    // worn by the next address.
    //
    // A navigation that never ends leaves the title alone, and that is what
    // makes a sheet harmless: its address is a redirect back to the page you
    // are on, which Angular skips.
    this.router.events.subscribe((event) => {
      if (event instanceof NavigationStart) this.named = false;
      if (event instanceof NavigationEnd && !this.named) this.pageTitle.set(undefined);
    });

    // The project and the organizations arrive after the page has already named
    // itself, so the title is written from what is known now rather than once.
    effect(() => {
      const parts = [this.pageTitle(), ...this.ownerNames(), this.organizationName()].filter(
        Boolean,
      );
      this.title.setTitle(
        parts.length === 0 ? this.DEFAULT_TITLE : [...parts, this.PRODUCT].join(this.SEPARATOR),
      );
    });
  }

  setTitle(pageTitle?: string): void {
    this.named = true;
    this.pageTitle.set(pageTitle);
  }

  setDescription(description: string): void {
    this.meta.updateTag({ name: 'description', content: description });
  }
}
