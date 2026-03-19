export const POSTMESSAGE_VERSION = 1;

export type HostMessage =
  | {
      type: 'fundament:init';
      version: number;
      theme: 'light' | 'dark';
      pluginName: string;
      crdKind: string;
      view: 'list' | 'detail';
    }
  | {
      type: 'fundament:theme-changed';
      theme: 'light' | 'dark';
    };

export type PluginMessage =
  | {
      type: 'plugin:ready';
      version: number;
    }
  | {
      type: 'plugin:resize';
      height: number;
    }
  | {
      type: 'plugin:navigate';
      path: string;
    };
