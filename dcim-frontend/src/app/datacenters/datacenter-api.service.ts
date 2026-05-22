import { Injectable, inject } from '@angular/core';
import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
import type { Site } from '../../generated/v1/site_pb';
import type { Room as ProtoRoom } from '../../generated/v1/room_pb';
import type { RackRow as ProtoRackRow } from '../../generated/v1/rack_row_pb';
import type { Rack as ProtoRack } from '../../generated/v1/rack_pb';
import type {
  DatacenterInfo,
  DatacenterRack,
  DatacenterStatus,
  Room,
  RackRow,
} from './datacenter.model';
import { SITE_CLIENT, ROOM_CLIENT, RACK_ROW_CLIENT, RACK_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class DatacenterApiService {
  private readonly siteClient = inject(SITE_CLIENT);

  private readonly roomClient = inject(ROOM_CLIENT);

  private readonly rackRowClient = inject(RACK_ROW_CLIENT);

  private readonly rackClient = inject(RACK_CLIENT);

  static mapSite(s: Site): DatacenterInfo {
    return {
      id: s.id,
      name: s.name,
      fullName: s.fullName,
      address: s.address,
      city: s.city,
      country: s.country,
      tier: (parseInt(s.tier, 10) || 3) as 1 | 2 | 3 | 4,
      established: s.established ? timestampDate(s.established).getFullYear() : 0,
      status: DatacenterApiService.mapStatus(s.status),
      floorSqm: s.floorSqm,
      // Power, cooling and PUE are not modelled by the API; kept at 0 and not
      // shown on the datacenters page.
      powerCapacityKw: 0,
      coolingCapacityKw: 0,
      pue: 0,
    };
  }

  private static mapStatus(status: string): DatacenterStatus {
    return status === 'degraded' || status === 'maintenance' ? status : 'operational';
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

  static mapRack(r: ProtoRack): DatacenterRack {
    return { id: r.id, rowId: r.rowId, name: r.name, totalU: r.totalUnits };
  }

  listSites() {
    return this.siteClient.listSites({});
  }

  getSite(id: string) {
    return this.siteClient.getSite({ id });
  }

  createSite(dc: DatacenterInfo) {
    return this.siteClient.createSite({
      name: dc.name,
      fullName: dc.fullName,
      address: dc.address,
      city: dc.city,
      country: dc.country,
      tier: String(dc.tier),
      floorSqm: dc.floorSqm,
      status: dc.status,
      ...(dc.established > 0
        ? { established: timestampFromDate(new Date(dc.established, 0, 1)) }
        : {}),
    });
  }

  updateSite(dc: DatacenterInfo) {
    return this.siteClient.updateSite({
      id: dc.id,
      name: dc.name,
      fullName: dc.fullName,
      address: dc.address,
      city: dc.city,
      country: dc.country,
      tier: String(dc.tier),
      floorSqm: dc.floorSqm,
      status: dc.status,
      ...(dc.established > 0
        ? { established: timestampFromDate(new Date(dc.established, 0, 1)) }
        : {}),
    });
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

  listRacks(rowId: string) {
    return this.rackClient.listRacks({ rowId });
  }

  createRack(rowId: string, name: string, totalUnits: number) {
    return this.rackClient.createRack({ rowId, name, totalUnits, positionInRow: 0 });
  }

  updateRack(id: string, name: string, totalUnits: number) {
    return this.rackClient.updateRack({ id, name, totalUnits });
  }

  deleteRack(id: string) {
    return this.rackClient.deleteRack({ id });
  }
}
