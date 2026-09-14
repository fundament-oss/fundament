import { TestBed } from '@angular/core/testing';
import { Transport } from '@connectrpc/connect';
import App from './app';
import { ConfigService, type AppConfiguration } from './config.service';
import {
  AUTHN_TRANSPORT,
  ORGANIZATION_TRANSPORT,
  MARKETPLACE_TRANSPORT,
} from '../connect/connect.module';

// The shell injects service clients (MetricsHealthService among them), and every
// client token builds itself from one of these transports. Nothing here makes a
// call, so a transport that would throw if used is enough — the point is that the
// injector can build the component at all.
const unusedTransport = {
  unary: () => Promise.reject(new Error('no transport in tests')),
  stream: () => Promise.reject(new Error('no transport in tests')),
} as unknown as Transport;

// getConfig() throws until loadConfig() has run, which is an APP_INITIALIZER the
// TestBed does not execute. Empty URLs are what an environment with no demo and
// no marketplace deployed looks like.
const config: AppConfiguration = {
  authnApiUrl: '',
  organizationApiUrl: '',
  marketplaceApiUrl: '',
  kubeApiProxyUrl: '',
  pluginProxyUrl: '',
};

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        { provide: AUTHN_TRANSPORT, useValue: unusedTransport },
        { provide: ORGANIZATION_TRANSPORT, useValue: unusedTransport },
        { provide: MARKETPLACE_TRANSPORT, useValue: unusedTransport },
        { provide: ConfigService, useValue: { getConfig: () => config } as ConfigService },
      ],
    }).compileComponents();
  });

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    const app = fixture.componentInstance;
    expect(app).toBeTruthy();
  });
});
