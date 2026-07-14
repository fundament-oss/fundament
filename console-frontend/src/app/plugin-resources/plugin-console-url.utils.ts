/**
 * Builds the absolute URL for a plugin's console asset (`/console/<path>`)
 * served by the FUN-17 plugin-proxy on its dedicated origin.
 *
 * The URL includes the cluster the user is browsing — asset traffic lands on
 * that cluster's plugin pod, not on some arbitrary cluster the resolver
 * happened to pick. Otherwise one unlucky cluster ends up serving asset
 * requests for every plugin installation across the estate.
 *
 * Under FUN-17 the plugin iframe runs on a dedicated `plugin-proxy` origin —
 * cross-site with the console — so the browser refuses any parent DOM access
 * and applies the strict plugin CSP served by plugin-proxy.
 *
 * URL shape (matches plugin-proxy/pkg/assets/handler.go):
 *
 *   ${pluginProxyUrl}/clusters/${clusterId}/plugins/${pluginName}/${pluginVersion}/console/${path}
 */
export default function buildPluginConsoleUrl(args: {
  pluginProxyUrl: string;
  clusterId: string;
  pluginName: string;
  pluginVersion: string;
  path: string;
}): string {
  // Pre-built absolute URLs (e.g. /plugin-ui/demo/...) are passed through so
  // the bundled demo plugin still loads. Under FUN-17 the demo will move to
  // plugin-proxy too; that's a follow-up.
  if (/^https?:\/\//.test(args.path) || args.path.startsWith('/plugin-ui/')) {
    return args.path;
  }

  const base = args.pluginProxyUrl.replace(/\/$/, '');
  const consolePath = args.path.startsWith('/') ? args.path.slice(1) : args.path;
  return (
    `${base}/clusters/${encodeURIComponent(args.clusterId)}` +
    `/plugins/${encodeURIComponent(args.pluginName)}` +
    `/${encodeURIComponent(args.pluginVersion)}/console/${consolePath}`
  );
}

/**
 * PluginLike is the minimum plugin shape buildCustomUIUrl needs. Kept narrow
 * so callers with any registry-entry variant (list/detail/create) can pass
 * through without an adapter.
 */
export interface PluginLike {
  name: string;
  installationVersion: string;
  customComponents?: Record<string, { list?: string; detail?: string; create?: string } | undefined>;
}

export type CustomUIView = 'list' | 'detail' | 'create';

/**
 * buildCustomUIUrl returns the plugin-proxy URL for a resource kind's custom
 * component slot (list/detail/create), or `null` if the plugin doesn't
 * declare one. Extracts the null-guard + config-lookup + URL-build sequence
 * that was previously duplicated across resource-list / resource-detail /
 * resource-create components.
 */
export function buildCustomUIUrl(args: {
  plugin: PluginLike | null | undefined;
  kind: string | undefined;
  view: CustomUIView;
  clusterId: string | null | undefined;
  pluginProxyUrl: string;
}): string | null {
  const { plugin, kind, view, clusterId, pluginProxyUrl } = args;
  if (!plugin || !kind || !clusterId) return null;
  const path = plugin.customComponents?.[kind]?.[view];
  if (!path) return null;
  return buildPluginConsoleUrl({
    pluginProxyUrl,
    clusterId,
    pluginName: plugin.name,
    pluginVersion: plugin.installationVersion,
    path,
  });
}
