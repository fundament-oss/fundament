import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { ORGANIZATION } from '../../connect/tokens';
import {
  GetOrganizationQuotasRequestSchema,
  type ClusterQuota,
  type NodeQuota,
} from '../../generated/v1/organization_pb';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

/** One line of the page: a quota next to what counts against it. */
export interface QuotaLine {
  /** What the quota is for: "Clusters", or a machine type in a region. */
  label: string;
  /** The region, for a node quota; the cluster line has none. */
  region: string | null;
  quota: number;
  inUse: number;
}

/** "3 of 5", with a word on the state the numbers leave the organization in. */
export function quotaText(line: QuotaLine): string {
  const noun = line.region === null ? 'clusters' : 'nodes';
  if (line.quota === 0) return `${line.inUse} in use, none allowed`;
  if (line.inUse > line.quota) return `${line.inUse} of ${line.quota} ${noun}, over the quota`;
  if (line.inUse === line.quota) return `${line.inUse} of ${line.quota} ${noun}, all used`;
  return `${line.inUse} of ${line.quota} ${noun}`;
}

export function clusterLine(clusters: ClusterQuota | undefined): QuotaLine {
  return { label: 'Clusters', region: null, quota: clusters?.quota ?? 0, inUse: clusters?.inUse ?? 0 };
}

export function nodeLine(node: NodeQuota): QuotaLine {
  return { label: node.machineType, region: node.region, quota: node.quota, inUse: node.inUse };
}

/**
 * What the organization may build and what it has, as a platform operator set
 * it. Read-only: there is nothing here to change, only to know, and who to ask.
 */
@Component({
  selector: 'app-quotas',
  templateUrl: './quotas.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class QuotasComponent implements OnInit {
  private titleService = inject(TitleService);

  private organizationClient = inject(ORGANIZATION);

  protected pageNav = inject(PageNavService);

  protected loading = signal(true);

  protected loadError = signal<string | null>(null);

  protected clusters = signal<QuotaLine>(clusterLine(undefined));

  protected nodes = signal<QuotaLine[]>([]);

  /** No node quota allows a pool: nothing listed, quotas an operator set to
   *  zero, or only pools from before quotas existed, which show as quota 0.
   *  The API reports a missing row and a zero alike; either way no pool can be
   *  added, and the text says that rather than claiming nothing was set. */
  protected noNodeQuota = computed(() => this.nodes().every((line) => line.quota === 0));

  /** Whether any quota is exceeded: an operator lowered one under what runs,
   *  and the page should say that this is allowed and what it means. */
  protected anyOver = computed(() =>
    [this.clusters(), ...this.nodes()].some((line) => line.inUse > line.quota),
  );

  protected readonly quotaText = quotaText;

  constructor() {
    this.titleService.setTitle('Quotas');
  }

  async ngOnInit() {
    try {
      const response = await firstValueFrom(
        this.organizationClient.getOrganizationQuotas(
          create(GetOrganizationQuotasRequestSchema, {}),
        ),
      );
      this.clusters.set(clusterLine(response.clusters));
      this.nodes.set(response.nodes.map(nodeLine));
    } catch {
      this.loadError.set('Failed to load the quotas.');
    } finally {
      this.loading.set(false);
    }
  }
}
