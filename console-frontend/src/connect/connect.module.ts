// Adapted from: https://github.com/connectrpc/examples-es/blob/main/angular/src/connect/connect.module.ts

import { inject, InjectionToken, Provider } from '@angular/core';
import { Interceptor, Transport } from '@connectrpc/connect';
import { createConnectTransport, createGrpcWebTransport } from '@connectrpc/connect-web';
import { DescService } from '@bufbuild/protobuf';
import { createObservableClient, ObservableClient } from './observable-client';

const TRANSPORT = new InjectionToken<Transport>('connect.transport');

export const INTERCEPTORS = new InjectionToken<Interceptor[]>('connect.interceptors', {
  factory: () => [],
});

// Create a named transport token
function createTransportToken(name: string): InjectionToken<Transport> {
  return new InjectionToken<Transport>(`connect.transport.${name}`);
}

// Named transports for different services
export const AUTHN_TRANSPORT = createTransportToken('authn');
export const ORGANIZATION_TRANSPORT = createTransportToken('organization');

export function createClientToken<T extends DescService>(
  service: T,
  transportToken: InjectionToken<Transport> = TRANSPORT,
): InjectionToken<ObservableClient<T>> {
  return new InjectionToken(`client for ${service.typeName}`, {
    factory() {
      return createObservableClient(service, inject(transportToken));
    },
  });
}

export function provideConnect(
  options: Omit<Parameters<typeof createConnectTransport>[0], 'interceptors'>,
): Provider[] {
  return [
    {
      provide: TRANSPORT,
      useFactory: (interceptors: Interceptor[]) =>
        createConnectTransport({
          ...options,
          interceptors,
        }),
      deps: [INTERCEPTORS],
    },
  ];
}

export function provideGrpcWeb(
  options: Omit<Parameters<typeof createGrpcWebTransport>[0], 'interceptors'>,
): Provider[] {
  return [
    {
      provide: TRANSPORT,
      useFactory: (interceptors: Interceptor[]) =>
        createGrpcWebTransport({
          ...options,
          interceptors,
        }),
      deps: [INTERCEPTORS],
    },
  ];
}
