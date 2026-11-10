import type { PluginSummary } from '../../generated/catalog/v1/catalog_pb';
import type { Publisher } from '../../generated/marketplace/v1/common_pb';

interface InstalledRef {
  spec: { definitionRef: { organizationName: string; pluginName: string } };
}

/** The listings behind a cluster's installations. A plugin name is unique only
 *  per publisher, so both halves must match. */
export default function installedListings(
  plugins: PluginSummary[],
  publishers: Publisher[],
  installations: InstalledRef[],
): PluginSummary[] {
  const publisherNames = new Map(publishers.map((p) => [p.id, p.name]));
  const installed = new Set(
    installations.map(
      (i) => `${i.spec.definitionRef.organizationName}/${i.spec.definitionRef.pluginName}`,
    ),
  );
  return plugins.filter((p) => {
    const publisher = publisherNames.get(p.organizationId);
    return publisher !== undefined && installed.has(`${publisher}/${p.name}`);
  });
}
