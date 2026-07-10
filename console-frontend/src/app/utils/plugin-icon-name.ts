const getPluginIconName = (pluginName: string): string =>
  pluginName.toLowerCase().replace(/[^a-z]+/g, '-');

export default getPluginIconName;
