// Adapted from: https://github.com/connectrpc/examples-es/blob/main/angular/src/connect/connect.module.ts

import { inject, InjectionToken } from '@angular/core';
import { Transport } from '@connectrpc/connect';
import { DescService } from '@bufbuild/protobuf';
import { createObservableClient, ObservableClient } from './observable-client';

// Create a named transport token
function createTransportToken(name: string): InjectionToken<Transport> {
  return new InjectionToken<Transport>(`connect.transport.${name}`);
}

// Named transports for different services
export const AUTHN_TRANSPORT = createTransportToken('authn');
export const ORGANIZATION_TRANSPORT = createTransportToken('organization');
export const MARKETPLACE_TRANSPORT = createTransportToken('marketplace');
// install.v1 shares the marketplace host but is credentialed (cookie plus
// Fun-Organization), so it gets its own transport (FUN-22).
export const INSTALL_TRANSPORT = createTransportToken('install');
// The public platform overview (FUN-23): anonymous, like the catalog, and
// called over GET so the response can be cached.
export const PLATFORM_TRANSPORT = createTransportToken('platform');

export function createClientToken<T extends DescService>(
  service: T,
  transportToken: InjectionToken<Transport>,
): InjectionToken<ObservableClient<T>> {
  return new InjectionToken(`client for ${service.typeName}`, {
    factory() {
      return createObservableClient(service, inject(transportToken));
    },
  });
}
