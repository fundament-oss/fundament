import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { FLEET_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import type {
  ClusterSummary,
  DriftFinding,
  FleetSummary,
  GetClusterResponse,
  OrganizationSummary,
} from '../../generated/fleet/v1/fleet_pb';

/** What the API's own default is, for the moment before the summary is in. */
const DEFAULT_STALE_AFTER_SECONDS = 45 * 60;

/**
 * The fleet as the Platform section shows it (FUN-23), held in signals.
 *
 * The fleet-wide reads (the summary, every organization, the drift findings)
 * load once per visit to the section and feed the menu as well as the
 * overview. One organization or one cluster loads when it is opened; a read
 * that a newer one overtook is dropped, so clicking through quickly cannot
 * leave the page showing the cluster before last.
 */
@Injectable({ providedIn: 'root' })
export default class PlatformApiService {
  private readonly client = inject(FLEET_CLIENT);

  readonly summary = signal<FleetSummary | undefined>(undefined);

  readonly organizations = signal<OrganizationSummary[]>([]);

  readonly findings = signal<DriftFinding[]>([]);

  readonly fleetLoaded = signal(false);

  readonly fleetError = signal<string | null>(null);

  readonly organization = signal<OrganizationSummary | undefined>(undefined);

  readonly clusters = signal<ClusterSummary[]>([]);

  readonly cluster = signal<GetClusterResponse | undefined>(undefined);

  readonly detailLoading = signal(false);

  readonly detailError = signal<string | null>(null);

  readonly staleAfterSeconds = computed(
    () => this.summary()?.staleAfterSeconds || DEFAULT_STALE_AFTER_SECONDS,
  );

  private detailRequest = 0;

  async loadFleet(): Promise<void> {
    this.fleetError.set(null);
    try {
      const [summary, organizations, findings] = await Promise.all([
        firstValueFrom(this.client.getFleetSummary({})),
        firstValueFrom(this.client.listOrganizations({})),
        firstValueFrom(this.client.listDriftFindings({})),
      ]);
      this.summary.set(summary.summary);
      this.organizations.set(organizations.organizations);
      this.findings.set(findings.findings);
      this.fleetLoaded.set(true);
    } catch (err) {
      this.fleetError.set(connectErrorMessage(err));
    }
  }

  async loadOrganization(id: string): Promise<void> {
    const request = this.startDetail();
    this.organization.set(undefined);
    this.clusters.set([]);
    try {
      const [organization, clusters] = await Promise.all([
        firstValueFrom(this.client.getOrganization({ organizationId: id })),
        firstValueFrom(this.client.listClusters({ organizationId: id })),
      ]);
      if (request !== this.detailRequest) return;
      this.organization.set(organization.organization);
      this.clusters.set(clusters.clusters);
    } catch (err) {
      if (request === this.detailRequest) this.detailError.set(connectErrorMessage(err));
    } finally {
      if (request === this.detailRequest) this.detailLoading.set(false);
    }
  }

  async loadCluster(id: string): Promise<void> {
    const request = this.startDetail();
    this.cluster.set(undefined);
    try {
      const cluster = await firstValueFrom(this.client.getCluster({ clusterId: id }));
      if (request !== this.detailRequest) return;
      this.cluster.set(cluster);
    } catch (err) {
      if (request === this.detailRequest) this.detailError.set(connectErrorMessage(err));
    } finally {
      if (request === this.detailRequest) this.detailLoading.set(false);
    }
  }

  private startDetail(): number {
    this.detailRequest += 1;
    this.detailLoading.set(true);
    this.detailError.set(null);
    return this.detailRequest;
  }
}
