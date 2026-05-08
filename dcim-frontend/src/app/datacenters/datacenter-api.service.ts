import { Injectable, inject } from '@angular/core';
import type { Site } from '../../generated/v1/site_pb';
import type { Room as ProtoRoom } from '../../generated/v1/room_pb';
import type { RackRow as ProtoRackRow } from '../../generated/v1/rack_row_pb';
import type { DatacenterInfo, Room, RackRow } from './datacenter.model';
import { SITE_CLIENT, ROOM_CLIENT, RACK_ROW_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class DatacenterApiService {
  private readonly siteClient = inject(SITE_CLIENT);

  private readonly roomClient = inject(ROOM_CLIENT);

  private readonly rackRowClient = inject(RACK_ROW_CLIENT);

  static mapSite(s: Site): Pick<DatacenterInfo, 'id' | 'name' | 'address'> {
    return { id: s.id, name: s.name, address: s.address };
  }

  static mapRoom(r: ProtoRoom): Room {
    return {
      id: r.id,
      siteId: r.siteId,
      name: r.name,
      floor: parseInt(r.floor, 10) || 0,
    };
  }

  static mapRackRow(rr: ProtoRackRow): RackRow {
    return {
      id: rr.id,
      roomId: rr.roomId,
      name: rr.name,
      positionX: rr.positionX,
      positionY: rr.positionY,
    };
  }

  listSites() {
    return this.siteClient.listSites({});
  }

  createSite(name: string, address: string) {
    return this.siteClient.createSite({ name, address });
  }

  updateSite(id: string, name: string, address: string) {
    return this.siteClient.updateSite({ id, name, address });
  }

  deleteSite(id: string) {
    return this.siteClient.deleteSite({ id });
  }

  listRooms(siteId: string) {
    return this.roomClient.listRooms({ siteId });
  }

  createRoom(siteId: string, name: string, floor: number) {
    return this.roomClient.createRoom({ siteId, name, floor: String(floor) });
  }

  updateRoom(id: string, name: string, floor: number) {
    return this.roomClient.updateRoom({ id, name, floor: String(floor) });
  }

  deleteRoom(id: string) {
    return this.roomClient.deleteRoom({ id });
  }

  listRackRows(roomId: string) {
    return this.rackRowClient.listRackRows({ roomId });
  }

  createRackRow(roomId: string, name: string, positionX: number, positionY: number) {
    return this.rackRowClient.createRackRow({ roomId, name, positionX, positionY });
  }

  updateRackRow(id: string, name: string, positionX: number, positionY: number) {
    return this.rackRowClient.updateRackRow({ id, name, positionX, positionY });
  }

  deleteRackRow(id: string) {
    return this.rackRowClient.deleteRackRow({ id });
  }
}
