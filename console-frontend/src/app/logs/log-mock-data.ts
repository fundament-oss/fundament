import type { LogEntry, LogLevel } from './log.types';

const CLUSTERS = ['prod-eu-west', 'prod-us-east', 'staging'];

interface PodSpec {
  prefix: string;
  containers: string[];
}

const NS_PODS: Record<string, PodSpec[]> = {
  production: [
    { prefix: 'api', containers: ['api', 'envoy-proxy'] },
    { prefix: 'web', containers: ['nginx', 'web-app'] },
    { prefix: 'worker', containers: ['worker'] },
    { prefix: 'scheduler', containers: ['scheduler'] },
  ],
  staging: [
    { prefix: 'api', containers: ['api'] },
    { prefix: 'web', containers: ['nginx'] },
    { prefix: 'worker', containers: ['worker'] },
  ],
  monitoring: [
    { prefix: 'grafana', containers: ['grafana'] },
    { prefix: 'alertmanager', containers: ['alertmanager'] },
    { prefix: 'prometheus', containers: ['prometheus'] },
  ],
  'kube-system': [
    { prefix: 'coredns', containers: ['coredns'] },
    { prefix: 'kube-proxy', containers: ['kube-proxy'] },
  ],
};

const CLUSTER_NAMESPACES: Record<string, string[]> = {
  'prod-eu-west': ['production', 'monitoring', 'kube-system'],
  'prod-us-east': ['production', 'monitoring', 'kube-system'],
  staging: ['staging', 'kube-system'],
};

const LEVEL_MESSAGES: Record<LogLevel, Array<{ msg: string; fields: Record<string, unknown> }>> = {
  ERROR: [
    {
      msg: 'connection refused: dial tcp 10.0.1.4:5432: connect: connection refused',
      fields: { 'error.code': 'ECONNREFUSED', 'db.host': '10.0.1.4', 'db.port': 5432, attempt: 3 },
    },
    {
      msg: 'OOMKilled: container exceeded memory limit (512Mi)',
      fields: { 'memory.limit': '512Mi', 'memory.used': '548Mi', container: 'api' },
    },
    {
      msg: 'panic: runtime error: index out of range [3] with length 2',
      fields: { 'goroutine': 1, 'file': 'handlers/auth.go', 'line': 147 },
    },
    {
      msg: 'TLS handshake timeout connecting to upstream api.internal:443',
      fields: { 'upstream': 'api.internal:443', 'timeout_ms': 30000 },
    },
    {
      msg: 'ImagePullBackOff: failed to pull image registry.internal/app:v2.3.1',
      fields: { 'image': 'registry.internal/app:v2.3.1', 'reason': 'unauthorized' },
    },
    {
      msg: 'FATAL: database connection pool exhausted (max=50)',
      fields: { 'pool.size': 50, 'pool.active': 50, 'pool.waiting': 12 },
    },
  ],
  WARN: [
    {
      msg: 'slow query detected: SELECT * FROM events took 4.2s (threshold: 1s)',
      fields: { 'query.duration_ms': 4200, 'threshold_ms': 1000, 'table': 'events' },
    },
    {
      msg: 'memory usage at 87% of limit (448Mi/512Mi)',
      fields: { 'memory.used': '448Mi', 'memory.limit': '512Mi', 'pct': 87 },
    },
    {
      msg: 'retrying failed request to cache.internal:6379 (attempt 2/3)',
      fields: { 'host': 'cache.internal:6379', 'attempt': 2, 'max_attempts': 3 },
    },
    {
      msg: 'queue depth 847 exceeds warning threshold of 500',
      fields: { 'queue': 'task-queue', 'depth': 847, 'threshold': 500 },
    },
    {
      msg: 'certificate will expire in 14 days: *.internal.example.com',
      fields: { 'cert.domain': '*.internal.example.com', 'days_remaining': 14 },
    },
    {
      msg: 'rate limit approaching: 4823/5000 requests in current window',
      fields: { 'current': 4823, 'limit': 5000, 'window_s': 60 },
    },
  ],
  INFO: [
    {
      msg: 'GET /api/v1/clusters 200 OK (34ms)',
      fields: { 'http.method': 'GET', 'http.path': '/api/v1/clusters', 'http.status': 200, 'duration_ms': 34 },
    },
    {
      msg: 'POST /api/v1/namespaces 201 Created (89ms)',
      fields: { 'http.method': 'POST', 'http.path': '/api/v1/namespaces', 'http.status': 201, 'duration_ms': 89 },
    },
    {
      msg: 'GET /health 200 OK (2ms)',
      fields: { 'http.method': 'GET', 'http.path': '/health', 'http.status': 200, 'duration_ms': 2 },
    },
    {
      msg: 'worker started: processing task-queue (concurrency=4)',
      fields: { 'queue': 'task-queue', 'concurrency': 4, 'worker_id': 'w-7f3a2' },
    },
    {
      msg: 'cache miss for key user:profile:f8c3de3d (fetching from database)',
      fields: { 'cache.key': 'user:profile:f8c3de3d', 'cache.result': 'miss' },
    },
    {
      msg: 'scheduled job reconcile-clusters completed in 1.23s (42 clusters processed)',
      fields: { 'job': 'reconcile-clusters', 'duration_s': 1.23, 'processed': 42 },
    },
    {
      msg: 'DELETE /api/v1/plugins/cert-manager 204 No Content (156ms)',
      fields: { 'http.method': 'DELETE', 'http.path': '/api/v1/plugins/cert-manager', 'http.status': 204, 'duration_ms': 156 },
    },
    {
      msg: 'user authenticated: admin@example.com via OIDC (token TTL: 3600s)',
      fields: { 'user.email': 'admin@example.com', 'auth.method': 'OIDC', 'token.ttl_s': 3600 },
    },
  ],
  DEBUG: [
    {
      msg: 'DB query: SELECT id, name FROM clusters WHERE org_id=$1 (2ms)',
      fields: { 'db.query': 'SELECT id, name FROM clusters WHERE org_id=$1', 'db.duration_ms': 2 },
    },
    {
      msg: 'cache hit for key cluster:summary:a1b2c3 (TTL remaining: 47s)',
      fields: { 'cache.key': 'cluster:summary:a1b2c3', 'cache.result': 'hit', 'ttl_s': 47 },
    },
    {
      msg: 'goroutine pool: 12 active / 48 idle / 60 max',
      fields: { 'goroutines.active': 12, 'goroutines.idle': 48, 'goroutines.max': 60 },
    },
    {
      msg: 'reconcile loop tick: checking 15 deployments for drift',
      fields: { 'deployments': 15, 'tick': 'reconcile-loop' },
    },
  ],
};

const LEVEL_WEIGHTS: [LogLevel, number][] = [
  ['ERROR', 0.12],
  ['WARN', 0.18],
  ['INFO', 0.58],
  ['DEBUG', 0.12],
];

function pickLevel(seed: number): LogLevel {
  const r = ((seed * 9301 + 49297) % 233280) / 233280;
  let acc = 0;
  for (const [level, weight] of LEVEL_WEIGHTS) {
    acc += weight;
    if (r < acc) return level;
  }
  return 'INFO';
}

function seededInt(seed: number, max: number): number {
  return Math.abs((seed * 1103515245 + 12345) & 0x7fffffff) % max;
}

function generatePodId(seed: number): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  const a = chars[seededInt(seed, chars.length)];
  const b = chars[seededInt(seed * 7, chars.length)];
  const c = chars[seededInt(seed * 13, chars.length)];
  const d = chars[seededInt(seed * 17, chars.length)];
  const e = chars[seededInt(seed * 19, chars.length)];
  return `${a}${b}${c}${d}${e}`;
}

export function generateMockLogs(count: number): LogEntry[] {
  const now = new Date();
  const logs: LogEntry[] = [];
  const THREE_HOURS_MS = 3 * 60 * 60 * 1000;

  for (let i = 0; i < count; i++) {
    const seed = i * 31337 + 42;
    const cluster = CLUSTERS[seededInt(seed, CLUSTERS.length)];
    const namespaces = CLUSTER_NAMESPACES[cluster];
    const namespace = namespaces[seededInt(seed * 3, namespaces.length)];
    const pods = NS_PODS[namespace];
    const podSpec = pods[seededInt(seed * 5, pods.length)];
    const container = podSpec.containers[seededInt(seed * 7, podSpec.containers.length)];
    const podId = generatePodId(seed);
    const pod = `${podSpec.prefix}-${podId}`;
    const level = pickLevel(seed);
    const messages = LEVEL_MESSAGES[level];
    const messageEntry = messages[seededInt(seed * 11, messages.length)];
    const ageMs = seededInt(seed * 23, THREE_HOURS_MS);
    const timestamp = new Date(now.getTime() - ageMs);

    logs.push({
      id: `mock-${i}`,
      timestamp,
      level,
      cluster,
      namespace,
      pod,
      container,
      message: messageEntry.msg,
      fields: {
        ...messageEntry.fields,
        'trace_id': `${generatePodId(seed * 37)}${generatePodId(seed * 41)}`,
        'span_id': generatePodId(seed * 43),
      },
    });
  }

  return logs.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
}

export function generateLiveTailEntry(cluster: string, namespace: string): LogEntry {
  const seed = Date.now() % 999983;
  const ns = namespace || 'production';
  const cl = cluster || 'prod-eu-west';
  const pods = NS_PODS[ns] ?? NS_PODS['production'];
  const podSpec = pods[seededInt(seed, pods.length)];
  const container = podSpec.containers[seededInt(seed * 3, podSpec.containers.length)];
  const pod = `${podSpec.prefix}-${generatePodId(seed)}`;
  const level = pickLevel(seed);
  const messages = LEVEL_MESSAGES[level];
  const messageEntry = messages[seededInt(seed * 7, messages.length)];

  return {
    id: `live-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    timestamp: new Date(),
    level,
    cluster: cl,
    namespace: ns,
    pod,
    container,
    message: messageEntry.msg,
    fields: {
      ...messageEntry.fields,
      'trace_id': `${generatePodId(seed * 37)}${generatePodId(seed * 41)}`,
      'span_id': generatePodId(seed * 43),
    },
  };
}
