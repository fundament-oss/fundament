export type Theme = 'light' | 'dark';

export interface ResourceContext {
  name: string;
  namespace?: string;
}

export type HostMessage =
  | {
      type: 'fundament:init';
      protocolVersion: 1;
      theme: Theme;
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail' | 'create';
      resource?: ResourceContext;
      namespaces?: string[];
      // FUN-17: plugins call kube-api-proxy directly with the token below.
      // Both fields are surfaced in the SDK's `fundament.init` so plugin JS can
      // build fetch URLs without knowing about the host.
      kubeApiProxyUrl: string;
      clusterId: string;
      token: string;
      tokenExpiresAt: number;
    }
  | {
      type: 'fundament:theme-changed';
      theme: Theme;
    }
  | {
      type: 'fundament:token-refreshed';
      token: string;
      tokenExpiresAt: number;
    }
  | {
      type: 'fundament:auth-failed';
      reason: 'mint_failed' | 'unauthorized' | 'revoked';
    };

export type PluginMessage =
  | { type: 'plugin:ready' }
  | { type: 'plugin:resize'; height: number }
  | { type: 'plugin:navigate'; name: string; namespace?: string }
  | { type: 'plugin:create' }
  | { type: 'plugin:navigate-back' }
  // Sent when the plugin sees an upstream 401. The host cancels the
  // pending refresh timer (scheduled off the previous token's expiresAt)
  // and mints a new token immediately; the resulting fundament:token-refreshed
  // unblocks any getToken() waiters on the plugin side. See Fix #11 in
  // the FUN-17 review — closes the "SDK 401 retry hangs forever" gap.
  | { type: 'plugin:request-token-refresh' };
