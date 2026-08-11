import { Injectable, computed, effect, inject, signal } from '@angular/core';
import { Title, Meta } from '@angular/platform-browser';
import { NavigationEnd, NavigationStart, Router } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { filter, map } from 'rxjs/operators';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';

/**
 * The document title, specific first and general last, with a middot between the
 * parts: someone with a screen full of tabs reads the first words.
 *
 *     Clusters · Fundament
 *     Limits · burgerzaken · Fundament
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

  /** The project or cluster the page belongs to, read from the address rather
   *  than passed down: every page under it would otherwise have to name it
   *  again, and the one that loads last would win. */
  private ownerName = computed(() => {
    const url = this.url();

    const projectId = url.match(/^\/projects\/([^/?#]+)/)?.[1];
    if (projectId) {
      const project = this.organizationData.getProjectById(projectId)?.project;
      return project?.alias || project?.name || null;
    }

    const clusterId = url.match(/^\/clusters\/([^/?#]+)/)?.[1];
    if (clusterId) {
      // The summaries rather than the nested organization data: that list is
      // lightweight and is the one the console loads for the current
      // organization, so it is filled long before the other.
      const summary = this.organizationData
        .clusterSummaries()
        .find((cluster) => cluster.id === clusterId);
      return summary?.name || this.organizationData.getClusterById(clusterId)?.cluster.name || null;
    }

    return null;
  });

  /** Empty until there is a second organization: see the class comment. */
  private organizationName = computed(() => {
    const organizations = this.organizationData.organizations();
    if (organizations.length < 2) return null;

    const current = this.organizationContext.currentOrganizationId();
    const organization = current ? this.organizationData.getOrganizationById(current) : undefined;
    return organization?.alias || organization?.name || null;
  });

  constructor() {
    // Leaving drops the page name, so a route without a component of its own
    // does not keep wearing the title of wherever you came from. On the way out
    // rather than on arrival: a page names itself while it is being activated,
    // which is before the navigation ends.
    this.router.events
      .pipe(filter((event) => event instanceof NavigationStart))
      .subscribe(() => this.pageTitle.set(undefined));

    // The project and the organizations arrive after the page has already named
    // itself, so the title is written from what is known now rather than once.
    effect(() => {
      const parts = [this.pageTitle(), this.ownerName(), this.organizationName()].filter(Boolean);
      this.title.setTitle(
        parts.length === 0 ? this.DEFAULT_TITLE : [...parts, this.PRODUCT].join(this.SEPARATOR),
      );
    });
  }

  setTitle(pageTitle?: string): void {
    this.pageTitle.set(pageTitle);
  }

  setDescription(description: string): void {
    this.meta.updateTag({ name: 'description', content: description });
  }
}
