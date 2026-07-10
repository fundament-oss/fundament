export const getPluginIconName = (pluginName: string): string =>
  pluginName.toLowerCase().replace(/[^a-z]+/g, '-');
