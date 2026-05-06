import { Injectable, inject } from '@angular/core';
import { RACK_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class RackApiService {
  private readonly client = inject(RACK_CLIENT);

  createRack(name: string, totalUnits: number, rowId: string) {
    return this.client.createRack({ rowId, name, totalUnits, positionInRow: 0 });
  }

  updateRack(id: string, name: string, totalUnits: number) {
    return this.client.updateRack({ id, name, totalUnits });
  }

  deleteRack(id: string) {
    return this.client.deleteRack({ id });
  }
}
