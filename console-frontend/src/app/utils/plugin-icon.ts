/** Where a plugin's logo lives, derived from its install identifier. The
 *  catalogue, the cluster page and the plugin picker all draw the same logo, so
 *  the derivation belongs in one place: a plugin that renames itself should
 *  either keep its logo everywhere or lose it everywhere. */
export default function pluginIconSrc(plugin: { name: string }): string {
  return `/img/plugins/${plugin.name.toLowerCase().replace(/[^a-z]+/g, '-')}.svg`;
}
