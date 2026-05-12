import type { KubeResource } from '../types';

export type Theme = 'light' | 'dark';

export interface ResourceContext {
  name: string;
  namespace?: string;
}

export type HostMessage =
  | {
      type: 'fundament:init';
      theme: Theme;
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail';
      resource?: ResourceContext;
    }
  | {
      type: 'fundament:theme-changed';
      theme: Theme;
    }
  | {
      type: 'fundament:k8s:result';
      requestId: string;
      ok: true;
      items?: KubeResource[];
      item?: KubeResource;
    }
  | {
      type: 'fundament:k8s:result';
      requestId: string;
      ok: false;
      error: string;
      status?: number;
    };

export interface K8sListRequest {
  type: 'plugin:k8s:list';
  requestId: string;
  group: string;
  version: string;
  resource: string;
  namespace?: string;
}

export interface K8sGetRequest {
  type: 'plugin:k8s:get';
  requestId: string;
  group: string;
  version: string;
  resource: string;
  name: string;
  namespace?: string;
}

export type PluginMessage =
  | { type: 'plugin:ready' }
  | { type: 'plugin:resize'; height: number }
  | { type: 'plugin:navigate'; name: string; namespace?: string }
  | K8sListRequest
  | K8sGetRequest;
