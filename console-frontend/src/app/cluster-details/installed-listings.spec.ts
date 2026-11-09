import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { PluginSummarySchema } from '../../generated/catalog/v1/catalog_pb';
import { PublisherSchema } from '../../generated/marketplace/v1/common_pb';
import installedListings from './installed-listings';

const ref = (organizationName: string, pluginName: string) => ({
  spec: { definitionRef: { organizationName, pluginName } },
});

describe('installedListings', () => {
  it('matches installations on organization and plugin name', () => {
    const acmeGrafana = create(PluginSummarySchema, {
      id: 'p-a',
      name: 'grafana',
      organizationId: 'org-a',
    });
    const otherGrafana = create(PluginSummarySchema, {
      id: 'p-b',
      name: 'grafana',
      organizationId: 'org-b',
    });
    const publishers = [
      create(PublisherSchema, { id: 'org-a', name: 'acme' }),
      create(PublisherSchema, { id: 'org-b', name: 'other' }),
    ];

    expect(
      installedListings([acmeGrafana, otherGrafana], publishers, [ref('acme', 'grafana')]),
    ).toEqual([acmeGrafana]);
  });

  it('leaves out a listing whose publisher is unknown', () => {
    const orphan = create(PluginSummarySchema, {
      id: 'p-x',
      name: 'grafana',
      organizationId: 'org-x',
    });

    expect(installedListings([orphan], [], [ref('acme', 'grafana')])).toEqual([]);
  });
});
