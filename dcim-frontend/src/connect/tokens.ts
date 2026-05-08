import { InjectionToken, inject } from '@angular/core';
import { Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { DescService } from '@bufbuild/protobuf';
import { createObservableClient, ObservableClient } from './observable-client';
import { ConfigService } from '../app/config.service';
import { AssetService } from '../generated/v1/asset_pb';
import { CatalogService } from '../generated/v1/catalog_pb';
import { PhysicalConnectionService } from '../generated/v1/connection_pb';
import {
  LogicalDesignService,
  LogicalDeviceService,
  LogicalConnectionService,
  LogicalDeviceLayoutService,
} from '../generated/v1/design_pb';
import { RackService } from '../generated/v1/rack_pb';
import { SiteService } from '../generated/v1/site_pb';
import { RoomService } from '../generated/v1/room_pb';
import { RackRowService } from '../generated/v1/rack_row_pb';

export const DCIM_TRANSPORT = new InjectionToken<Transport>('dcim-transport', {
  providedIn: 'root',
  factory: () => {
    const config = inject(ConfigService).getConfig();
    return createConnectTransport({ baseUrl: config.apiUrl });
  },
});

function createClientToken<T extends DescService>(service: T) {
  return new InjectionToken<ObservableClient<T>>(`dcim-client-${service.typeName}`, {
    providedIn: 'root',
    factory: () => createObservableClient(service, inject(DCIM_TRANSPORT)),
  });
}

export const ASSET_CLIENT = createClientToken(AssetService);
export const CATALOG_CLIENT = createClientToken(CatalogService);
export const DESIGN_CLIENT = createClientToken(LogicalDesignService);
export const DEVICE_CLIENT = createClientToken(LogicalDeviceService);
export const CONNECTION_CLIENT = createClientToken(LogicalConnectionService);
export const LAYOUT_CLIENT = createClientToken(LogicalDeviceLayoutService);
export const PHYSICAL_CONNECTION_CLIENT = createClientToken(PhysicalConnectionService);
export const RACK_CLIENT = createClientToken(RackService);
export const SITE_CLIENT = createClientToken(SiteService);
export const ROOM_CLIENT = createClientToken(RoomService);
export const RACK_ROW_CLIENT = createClientToken(RackRowService);
