import { Injectable, inject } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { ORGANIZATION } from '../connect/tokens';
import { GetOrganizationQuotasRequestSchema } from '../generated/v1/organization_pb';

/** A node quota next to the nodes counted against it: the sum of the autoscale
 *  maxima of all the organization's pools of that machine type, in every
 *  cluster in the region. */
export interface NodeQuotaUsage {
  quota: number;
  inUse: number;
}

/** Per machine type name. A machine type that is not in the map has a quota of
 *  0, as the API reads a missing quota: no pool of that type is allowed. */
export type NodeQuotas = ReadonlyMap<string, NodeQuotaUsage>;

// Not cached, unlike the region catalog: another user or cluster can use up the
// room at any time, so every form that bounds a pool by it reads it fresh.
@Injectable({ providedIn: 'root' })
export class NodeQuotaService {
  private client = inject(ORGANIZATION);

  async forRegion(region: string): Promise<NodeQuotas> {
    const response = await firstValueFrom(
      this.client.getOrganizationQuotas(create(GetOrganizationQuotasRequestSchema, {})),
    );
    return new Map(
      response.nodes
        .filter((node) => node.region === region)
        .map((node) => [node.machineType, { quota: node.quota, inUse: node.inUse }]),
    );
  }
}
