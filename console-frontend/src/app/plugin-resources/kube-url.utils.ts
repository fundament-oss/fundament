/**
 * Builds the absolute URL for a Kubernetes resource collection or object,
 * routed through kube-api-proxy.
 *
 * Handles the core group (served under `api/<version>`) vs. named API groups
 * (`apis/<group>/<version>`), an optional namespace segment for namespaced
 * resources, an optional object name for single-object (get) requests, and an
 * optional project filter (`fundamentProjectId`) for cluster-wide collection lists.
 *
 * Shared by the host-mediated iframe broker (custom plugin UIs) and the
 * generated read-only UI store, so resource URLs have one source of truth.
 */
export default function buildResourceUrl(
  base: string,
  clusterId: string,
  args: {
    group: string;
    version: string;
    resource: string;
    namespace?: string;
    name?: string;
    projectId?: string;
  },
): string {
  const groupPart =
    args.group === '' ? `api/${args.version}` : `apis/${args.group}/${args.version}`;
  const nsPart = args.namespace ? `/namespaces/${encodeURIComponent(args.namespace)}` : '';
  const namePart = args.name ? `/${encodeURIComponent(args.name)}` : '';
  const query = args.projectId ? `?fundamentProjectId=${encodeURIComponent(args.projectId)}` : '';
  return `${base}/clusters/${encodeURIComponent(clusterId)}/${groupPart}${nsPart}/${encodeURIComponent(args.resource)}${namePart}${query}`;
}
