export type HostMessage =
  | {
      type: 'fundament:init';
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
    }
  | {
      type: 'plugin:resize';
      height: number;
    }
  | {
      type: 'plugin:navigate';
      path: string;
    };
