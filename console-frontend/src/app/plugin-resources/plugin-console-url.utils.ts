/**
 * Builds the absolute URL for a plugin's console asset (`/console/<path>`)
 * served via the Kubernetes service proxy through kube-api-proxy.
 *
 * Plugins return relative paths from GetDefinition.customComponents (e.g.
 * `certificates-list.html`); the console expands them here so plugins do not
 * need to know about the kube-api-proxy URL structure.
 *
 * The `?host=...` query param is appended so the iframe can load the shared
 * /plugin-ui/ bundles (plugin SDK, NLDD Design System) from the Console origin
 * without hard-coding it in the plugin HTML. The value is `window.location.origin`.
 *
 * Console assets are served unauthenticated, so anyone can hand a victim a link with
 * a `?host=` of their choosing — which the plugin HTML would otherwise turn into a
 * `<script src>` on the asset-serving origin. kube-api-proxy is what prevents that:
 * it refuses a console asset whose `?host=` is not a configured Console origin, and
 * serves the assets under a CSP whose script-src names only those origins (see
 * kube-api-proxy/pkg/kube/plugin_console_assets.go). This builder always passes the
 * real origin, so it is unaffected.
 */
export default function buildPluginConsoleUrl(args: {
  kubeApiProxyUrl: string;
  clusterId: string;
  pluginName: string;
  path: string;
}): string {
  // Pre-built absolute URLs (e.g. /plugin-ui/demo/...) are passed through.
  if (/^https?:\/\//.test(args.path) || args.path.startsWith('/plugin-ui/')) {
    return args.path;
  }

  const base = args.kubeApiProxyUrl.replace(/\/$/, '');
  const namespace = `plugin-${args.pluginName}`;
  const service = `http:plugin-${args.pluginName}:8080`;
  const consolePath = args.path.startsWith('/') ? args.path.slice(1) : args.path;
  const url =
    `${base}/clusters/${encodeURIComponent(args.clusterId)}` +
    `/api/v1/namespaces/${encodeURIComponent(namespace)}` +
    `/services/${encodeURIComponent(service)}/proxy/console/${consolePath}`;
  const host = encodeURIComponent(window.location.origin);
  return `${url}?host=${host}`;
}
