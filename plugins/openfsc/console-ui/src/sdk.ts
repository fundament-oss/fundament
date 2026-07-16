// Types for the host-provided `window.fundament` SDK. Mirrors the runtime served
// from the Console origin at /plugin-ui/plugin-sdk.js (see
// console-frontend/src/plugin-sdk/plugin-sdk.ts). The dev preview ships a
// kubectl-backed stand-in with the same surface, minus onThemeChange.

export type Theme = 'light' | 'dark';

export interface ResourceContext {
  name: string | null;
  namespace?: string | null;
}

export interface InitContext {
  theme?: Theme;
  pluginName?: string;
  crdKind?: string;
  view?: 'list' | 'detail' | 'create';
  resource?: ResourceContext;
  namespaces?: string[];
}

export interface K8sRef {
  group: string;
  version: string;
  resource: string;
  namespace?: string;
}

export interface K8sGetRef extends K8sRef {
  name: string;
}

export interface KubeList<T = unknown> {
  items: T[];
}

export interface FundamentSdk {
  init: Promise<InitContext>;
  k8s: {
    list<T = unknown>(ref: K8sRef): Promise<KubeList<T>>;
    get<T = unknown>(ref: K8sGetRef): Promise<T>;
    create<T = unknown>(ref: K8sRef, body: unknown): Promise<T>;
  };
  onThemeChange?(cb: (theme: Theme) => void): () => void;
}

declare global {
  interface Window {
    fundament: FundamentSdk;
  }
}
