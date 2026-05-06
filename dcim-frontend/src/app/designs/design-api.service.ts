import { Injectable, inject } from '@angular/core';
import {
  LogicalDesignStatus,
  LogicalDeviceRole,
  LogicalConnectionType,
} from '../../generated/v1/design_pb';
import type {
  LogicalDesign as ProtoDesign,
  LogicalDevice as ProtoDevice,
  LogicalConnection as ProtoConn,
  LogicalDeviceLayout as ProtoLayout,
} from '../../generated/v1/design_pb';
import type {
  LogicalDesign,
  LogicalDevice,
  LogicalConnection,
  LogicalDeviceLayout,
  LogicalDesignStatus as FEStatus,
  LogicalDeviceRole as FERole,
  LogicalConnectionType as FEConnType,
} from './design.model';
import {
  DESIGN_CLIENT,
  DEVICE_CLIENT,
  CONNECTION_CLIENT,
  LAYOUT_CLIENT,
} from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class DesignApiService {
  private readonly designClient = inject(DESIGN_CLIENT);

  private readonly deviceClient = inject(DEVICE_CLIENT);

  private readonly connectionClient = inject(CONNECTION_CLIENT);

  private readonly layoutClient = inject(LAYOUT_CLIENT);

  static mapDesign(d: ProtoDesign): LogicalDesign {
    const statusMap: Record<number, FEStatus> = {
      [LogicalDesignStatus.DRAFT]: 'draft',
      [LogicalDesignStatus.ACTIVE]: 'active',
      [LogicalDesignStatus.ARCHIVED]: 'archived',
    };
    return {
      id: d.id,
      name: d.name,
      version: d.version,
      status: statusMap[d.status] ?? 'draft',
      created: d.created
        ? new Date(Number(d.created.seconds) * 1000).toISOString().slice(0, 10)
        : '',
    };
  }

  static mapDevice(d: ProtoDevice): LogicalDevice {
    const roleMap: Record<number, FERole> = {
      [LogicalDeviceRole.COMPUTE]: 'Compute',
      [LogicalDeviceRole.TOR]: 'ToR',
      [LogicalDeviceRole.SPINE]: 'Spine',
      [LogicalDeviceRole.CORE]: 'Core',
      [LogicalDeviceRole.PDU]: 'PDU',
      [LogicalDeviceRole.PATCH_PANEL]: 'Patch Panel',
      [LogicalDeviceRole.STORAGE]: 'Storage',
      [LogicalDeviceRole.FIREWALL]: 'Firewall',
      [LogicalDeviceRole.LOAD_BALANCER]: 'Load Balancer',
      [LogicalDeviceRole.CONSOLE_SERVER]: 'Console Server',
      [LogicalDeviceRole.CABLE_MANAGER]: 'Cable Manager',
      [LogicalDeviceRole.ADAPTER]: 'Adapter',
    };
    return {
      id: d.id,
      designId: d.designId,
      name: d.label,
      role: roleMap[d.role] ?? 'Compute',
      deviceCatalogId: d.deviceCatalogId || undefined,
    };
  }

  static mapConnection(c: ProtoConn): LogicalConnection {
    const typeMap: Record<number, FEConnType> = {
      [LogicalConnectionType.NETWORK]: 'network',
      [LogicalConnectionType.POWER]: 'power',
      [LogicalConnectionType.CONSOLE]: 'console',
    };
    return {
      id: c.id,
      designId: c.designId,
      sourceDeviceId: c.sourceDeviceId,
      sourcePortRole: c.sourcePortRole,
      targetDeviceId: c.targetDeviceId,
      targetPortRole: c.targetPortRole,
      connectionType: typeMap[c.connectionType] ?? 'network',
    };
  }

  static mapLayout(l: ProtoLayout): LogicalDeviceLayout {
    return { deviceId: l.deviceId, x: l.positionX, y: l.positionY };
  }

  private static toRoleEnum(role: FERole): LogicalDeviceRole {
    const map: Record<FERole, LogicalDeviceRole> = {
      Compute: LogicalDeviceRole.COMPUTE,
      ToR: LogicalDeviceRole.TOR,
      Spine: LogicalDeviceRole.SPINE,
      Core: LogicalDeviceRole.CORE,
      PDU: LogicalDeviceRole.PDU,
      'Patch Panel': LogicalDeviceRole.PATCH_PANEL,
      Storage: LogicalDeviceRole.STORAGE,
      Firewall: LogicalDeviceRole.FIREWALL,
      'Load Balancer': LogicalDeviceRole.LOAD_BALANCER,
      'Console Server': LogicalDeviceRole.CONSOLE_SERVER,
      'Cable Manager': LogicalDeviceRole.CABLE_MANAGER,
      Adapter: LogicalDeviceRole.ADAPTER,
    };
    return map[role] ?? LogicalDeviceRole.COMPUTE;
  }

  private static toStatusEnum(status: FEStatus): LogicalDesignStatus {
    const map: Record<FEStatus, LogicalDesignStatus> = {
      draft: LogicalDesignStatus.DRAFT,
      active: LogicalDesignStatus.ACTIVE,
      archived: LogicalDesignStatus.ARCHIVED,
    };
    return map[status] ?? LogicalDesignStatus.DRAFT;
  }

  private static toConnTypeEnum(type: FEConnType): LogicalConnectionType {
    const map: Record<FEConnType, LogicalConnectionType> = {
      network: LogicalConnectionType.NETWORK,
      power: LogicalConnectionType.POWER,
      console: LogicalConnectionType.CONSOLE,
    };
    return map[type] ?? LogicalConnectionType.NETWORK;
  }

  listDesigns() {
    return this.designClient.listDesigns({});
  }

  createDesign(name: string) {
    return this.designClient.createDesign({ name });
  }

  updateDesign(id: string, status: FEStatus) {
    return this.designClient.updateDesign({ id, status: DesignApiService.toStatusEnum(status) });
  }

  deleteDesign(id: string) {
    return this.designClient.deleteDesign({ id });
  }

  listDevices(designId: string) {
    return this.deviceClient.listDevices({ designId });
  }

  createDevice(designId: string, name: string, role: FERole) {
    return this.deviceClient.createDevice({
      designId,
      label: name,
      role: DesignApiService.toRoleEnum(role),
    });
  }

  updateDevice(id: string, name: string, role: FERole) {
    return this.deviceClient.updateDevice({
      id,
      label: name,
      role: DesignApiService.toRoleEnum(role),
    });
  }

  deleteDevice(id: string) {
    return this.deviceClient.deleteDevice({ id });
  }

  listConnections(designId: string) {
    return this.connectionClient.listConnections({ designId });
  }

  createConnection(c: LogicalConnection) {
    return this.connectionClient.createConnection({
      designId: c.designId,
      sourceDeviceId: c.sourceDeviceId,
      sourcePortRole: c.sourcePortRole,
      targetDeviceId: c.targetDeviceId,
      targetPortRole: c.targetPortRole,
      connectionType: DesignApiService.toConnTypeEnum(c.connectionType),
    });
  }

  updateConnection(c: LogicalConnection) {
    return this.connectionClient.updateConnection({
      id: c.id,
      sourcePortRole: c.sourcePortRole,
      targetPortRole: c.targetPortRole,
      connectionType: DesignApiService.toConnTypeEnum(c.connectionType),
    });
  }

  deleteConnection(id: string) {
    return this.connectionClient.deleteConnection({ id });
  }

  getLayout(designId: string) {
    return this.layoutClient.getLayout({ designId });
  }

  saveLayout(designId: string, layouts: LogicalDeviceLayout[]) {
    return this.layoutClient.saveLayout({
      designId,
      positions: layouts.map((l) => ({ deviceId: l.deviceId, positionX: l.x, positionY: l.y })),
    });
  }
}
