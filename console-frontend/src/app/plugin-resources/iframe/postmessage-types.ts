export type Theme = 'light' | 'dark';

export interface ResourceContext {
  name: string;
  namespace?: string;
}

/** The views a plugin page can ask the console to open in a sheet. */
export type SheetView = 'create' | 'edit' | 'detail';

/**
 * What a plugin page asks the console to open in a sheet. Names a page the
 * plugin's definition declares, never a URL: the console builds the URL from
 * customComponents[kind][view], the way it does for a routed page.
 */
export interface SheetRequest {
  /** The kind whose page opens; the opening page's own kind when absent. */
  kind?: string;
  view: SheetView;
  /** The resource the page is about. Required for edit and detail. */
  resource?: ResourceContext;
  /** Passed through to the sheet page's fundament:init as is. */
  params?: Record<string, string>;
}

export type HostMessage =
  | {
      type: 'fundament:init';
      protocolVersion: 1;
      theme: Theme;
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail' | 'create' | 'edit';
      // Whether the page fills a route or a sheet over one. A page that can be
      // both (a create page) leaves differently: it navigates back from a
      // route and asks the console to close the sheet.
      presentation: 'page' | 'sheet';
      resource?: ResourceContext;
      params?: Record<string, string>;
      namespaces?: string[];
      namespaceDisplayNames?: Record<string, string>;
      // FUN-17: plugins call kube-api-proxy directly with the token below.
      // Both fields are surfaced in the SDK's `fundament.init` so plugin JS can
      // build fetch URLs without knowing about the host.
      kubeApiProxyUrl: string;
      clusterId: string;
      // Set on a project page only.
      projectId?: string;
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
    }
  | {
      // Sent to the page under a sheet once the sheet saved something, so the
      // page can read its resources again. Carries nothing: the page reads
      // what changed itself, through its own token.
      type: 'fundament:resources-changed';
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
  | { type: 'plugin:request-token-refresh' }
  // Asks the console to open a sheet over the page. From a sheet, it replaces
  // that sheet. See SheetRequest.
  | ({ type: 'plugin:open-sheet' } & SheetRequest)
  // From a sheet: done. changed tells the page under it to read again.
  | { type: 'plugin:close-sheet'; changed?: boolean };
