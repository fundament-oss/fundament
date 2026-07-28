import { Injectable, inject } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { CLUSTER } from '../connect/tokens';
import { ListRegionsRequestSchema, Region } from '../generated/v1/cluster_pb';

export interface MachineTypeOption {
  value: string; // machine type name - exactly what CreateNodePoolRequest.machineType accepts
  label: string;
}

// Loads the region catalog (regions with their offered kubernetes versions and
// machine types) once and shares it between the add-cluster wizard and the
// cluster nodes page. The catalog is text-only: names are what the create
// endpoints accept.
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

  async getRegionByName(name: string): Promise<Region | undefined> {
    const regions = await this.getRegions();
    return regions.find((r) => r.name === name);
  }

  static machineTypeOptions(region: Region): MachineTypeOption[] {
    return region.machineTypes.map((mt) => ({
      value: mt.name,
      label: `${mt.name} (${mt.lcpu} lCPU, ${RegionCatalogService.formatMemory(mt.memory)})`,
    }));
  }

  // bigint-safe: derive GiB with one decimal without converting the raw byte
  // count through Number (which would lose precision above 2^53).
  private static formatMemory(bytes: bigint): string {
    const tenthGib = Number((bytes * 10n) / 1073741824n); // safe: catalog sizes are far below 2^53 tenths
    const whole = Math.floor(tenthGib / 10);
    const frac = tenthGib % 10;
    return frac === 0 ? `${whole} GiB RAM` : `${whole}.${frac} GiB RAM`;
  }
}
