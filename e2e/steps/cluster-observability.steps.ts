/**
 * Steps for the @cluster-observability feature: metrics and logs of a real
 * cluster through organization-api. Runs in the test-env VM
 * (`just test-env test`).
 */

import { Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { createServiceClient } from '../support/api/client.ts';
import { MetricsService } from '../support/generated/v1/metrics_pb.ts';
import { LogBackend, LogsService } from '../support/generated/v1/logs_pb.ts';
import { fixtureClusterId, session } from '../support/fixtures.ts';
import { pollUntil } from '../support/poll.ts';

// No cucumber timeouts: pollUntil enforces the minutes the feature names. A
// cluster's monitoring starts some minutes after the cluster runs.
Then(
  'within {int} minutes the metrics of the baseline cluster list its nodes and the namespace {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, namespace: string) {
    const s = session(this);
    const metrics = createServiceClient(MetricsService, s.organizationApiUrl, s.authToken, s.organizationId);
    const clusterId = await fixtureClusterId(this);
    await pollUntil(minutes * 60_000, `metrics of cluster ${clusterId} with nodes and pods in ${namespace}`, async () => {
      const r = await metrics.getClusterWorkloadMetrics({ clusterId });
      const ns = r.namespaces.find((n) => n.namespace === namespace);
      return !r.metricsUnavailable && r.nodes.length > 0 && ns && ns.pods > 0 ? r : undefined;
    });
  },
);

Then(
  'within {int} minutes the logs of the baseline cluster hold lines from the namespace {string}',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, namespace: string) {
    const s = session(this);
    const logs = createServiceClient(LogsService, s.organizationApiUrl, s.authToken, s.organizationId);
    const clusterId = await fixtureClusterId(this);
    const response = await pollUntil(minutes * 60_000, `logs of ${namespace} in cluster ${clusterId}`, async () => {
      const r = await logs.queryLogs({ clusterId, namespace, limit: 20 });
      return r.entries.length > 0 ? r : undefined;
    });
    expect(LogBackend[response.backend]).toBe(LogBackend[LogBackend.LOKI]);
    expect(response.entries.every((e) => e.namespace === namespace)).toBe(true);
  },
);
