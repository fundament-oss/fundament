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
  | { type: 'plugin:navigate-back' };
