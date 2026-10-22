import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { PLATFORM_OVERVIEW } from '../../connect/tokens';
import type { OrganizationOverview, PlatformTotals } from '../../generated/platform/v1/overview_pb';
import { TitleService } from '../title.service';
import { formatTime } from '../utils/date-format';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/card';
import '@nldd/design-system/collection';
import '@nldd/design-system/container';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/search-field';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/table';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-navigation-bar';

/** One of the tiles above the table. */
interface Tile {
  label: string;
  value: number;
}

/** Whether an organization matches what was typed, by name or by alias. */
export function matchesQuery(organization: OrganizationOverview, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    organization.name.toLowerCase().includes(q) || organization.alias.toLowerCase().includes(q)
  );
}

/** The tiles, in the order they are read: who, then what they run. */
export function tilesFor(totals: PlatformTotals | undefined): Tile[] {
  return [
    { label: 'Organizations', value: totals?.organizations ?? 0 },
    { label: 'Clusters', value: totals?.clusters ?? 0 },
    { label: 'Projects', value: totals?.projects ?? 0 },
    { label: 'Namespaces', value: totals?.namespaces ?? 0 },
  ];
}

/**
 * The public platform overview (FUN-23): every organization on Fundament with
 * how many clusters, projects and namespaces it runs. Anyone may read it,
 * signed in or not, so it renders without the console's shell and names
 * nothing inside an organization.
 */
@Component({
  selector: 'app-platform-overview',
  templateUrl: './platform-overview.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class PlatformOverviewComponent implements OnInit {
  private client = inject(PLATFORM_OVERVIEW);

  private titleService = inject(TitleService);

  isLoading = signal(true);

  error = signal<string | null>(null);

  organizations = signal<OrganizationOverview[]>([]);

  totals = signal<PlatformTotals | undefined>(undefined);

  asOf = signal('');

  query = signal('');

  tiles = computed(() => tilesFor(this.totals()));

  matching = computed(() => this.organizations().filter((o) => matchesQuery(o, this.query())));

  async ngOnInit(): Promise<void> {
    this.titleService.setTitle('Platform');
    try {
      const response = await firstValueFrom(this.client.getPlatformOverview({}));
      this.organizations.set(response.organizations);
      this.totals.set(response.totals);
      this.asOf.set(formatTime(response.generatedAt));
    } catch {
      this.error.set('The platform overview could not be loaded. Try again in a minute.');
    } finally {
      this.isLoading.set(false);
    }
  }
}
