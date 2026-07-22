import { Injectable, inject } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { CLUSTER } from '../connect/tokens';
import { ListRegionsRequestSchema, Region } from '../generated/v1/cluster_pb';

export interface MachineTypeOption {
  value: string; // catalog.region_machine_types id (CreateNodePoolRequest.regionMachineTypeId)
  label: string;
  name: string; // machine type name, e.g. c1-medium-x86
}

// Loads the region catalog (regions with their offered kubernetes versions and
// machine types) once and shares it between the add-cluster wizard and the
// cluster nodes page.
@Injectable({ providedIn: 'root' })
export class RegionCatalogService {
  private client = inject(CLUSTER);

  private regionsPromise: Promise<Region[]> | null = null;

  getRegions(): Promise<Region[]> {
    this.regionsPromise ??= firstValueFrom(
      this.client.listRegions(create(ListRegionsRequestSchema, {})),
    ).then(
      (response) => response.regions,
      (err) => {
        // Do not cache a failed load: let the next caller retry.
        this.regionsPromise = null;
        throw err;
      },
    );
    return this.regionsPromise;
  }

  async getRegionById(regionId: string): Promise<Region | undefined> {
    const regions = await this.getRegions();
    return regions.find((r) => r.id === regionId);
  }

  // Look up by the user-facing region name (clusters store it as their region).
  async getRegionByName(name: string): Promise<Region | undefined> {
    const regions = await this.getRegions();
    return regions.find((r) => r.name === name);
  }

  static machineTypeOptions(region: Region): MachineTypeOption[] {
    return region.machineTypes.map((mt) => ({
      value: mt.id,
      label: `${mt.name} (${mt.lcpu} lCPU, ${RegionCatalogService.formatMemory(mt.memory)})`,
      name: mt.name,
    }));
  }

  private static formatMemory(bytes: bigint): string {
    const gib = Number(bytes) / 1024 ** 3;
    const rounded = Number.isInteger(gib) ? gib.toString() : gib.toFixed(1);
    return `${rounded} GiB RAM`;
  }
}
