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
