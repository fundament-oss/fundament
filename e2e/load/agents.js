// Participants with AI agents, from `just e2e load-seed`.
//
//   agents    AGENTS VUs, one per participant. An agent step is 2–4 CLI
//             invocations in parallel. Each invocation exchanges the API key
//             and then makes one call, as functl and the kubectl exec plugin
//             do: lists and gets, or kubectl through kube-api-proxy. 5 % of
//             the steps create a project, wait until the creator may delete it
//             (the grant goes through the authz outbox), delete it and check
//             it is gone. 2 % ask for another team's organization, which must
//             be refused. A pause of THINK_MIN–THINK_MAX s follows; an LLM turn
//             usually takes longer, so the default errs towards more load.
//   consoles  CONSOLES open consoles (default: one per participant). Each
//             polls ListClusters every 5 s, as the dashboard does while a
//             cluster is changing, with a page view every fourth poll.
//
// Every response is checked against the participant's own organization:
// clusters, node pools, projects and API keys of another team must not show.
//
// Each request opens its own connection (REUSE=1 keeps them open): functl is
// a new process per invocation, so every call costs a TLS handshake.
import http from 'k6/http';
import { sleep } from 'k6';
import { SharedArray } from 'k6/data';
import { Trend } from 'k6/metrics';
import { RUN, URLS, baseOptions, bodyOf, check, headers, latency, track } from './lib.js';

const people = new SharedArray('people', () => JSON.parse(open(__ENV.PEOPLE)));
const duration = __ENV.DURATION || '10m';
const ramp = __ENV.RAMP || '2m';
const agents = Number(__ENV.AGENTS || people.length);
const consoles = Number(__ENV.CONSOLES ?? people.length);
const thinkMin = Number(__ENV.THINK_MIN || 0.5);
const thinkMax = Number(__ENV.THINK_MAX || 3);
if (agents > people.length || consoles > people.length) {
  throw new Error(`${Math.max(agents, consoles)} VUs for ${people.length} participants: seed more (just e2e load-seed <teams> <per-team>)`);
}

// Owner of every seeded API key, to spot a key of someone else in a list.
const keyOwner = Object.fromEntries(people.map((p) => [p.apiKeyId, p.email]));

// Time from creating a project until its creator may delete it.
const grantDelay = new Trend('grant_delay_ms', true);

const stages = (n) => [
  { duration: ramp, target: n },
  { duration, target: n },
];
const scenarios = {
  agents: { executor: 'ramping-vus', exec: 'agent', startVUs: 0, gracefulRampDown: '5s', stages: stages(agents) },
};
if (consoles > 0) {
  scenarios.consoles = { executor: 'ramping-vus', exec: 'consoleTab', startVUs: 0, gracefulRampDown: '5s', stages: stages(consoles) };
}

const calls = ['exchange', 'clusters', 'cluster', 'pools', 'projects', 'apikeys', 'kubectl', 'create', 'delete', 'deny', 'poll', 'userinfo', 'console'];
export const options = {
  ...baseOptions,
  noConnectionReuse: __ENV.REUSE !== '1',
  scenarios,
  thresholds: {
    http_req_failed: ['rate==0'],
    content_failures: ['count==0'],
    content_checks: ['count>0'],
    grant_delay_ms: ['p(95)<500', 'p(99)<2000'],
    ...latency(calls, Number(__ENV.P95_MS || 500)),
  },
};

const req = (name, path, token, org, body = {}, expect = [200]) => ({
  method: 'POST',
  url: `${URLS.org}/organization.v1.${path}`,
  body: JSON.stringify(body),
  params: { headers: headers(token, org), tags: { name }, timeout: '15s', responseCallback: http.expectedStatuses(...expect) },
});

function send(name, r, expect = [200]) {
  const res = http.request(r.method, r.url, r.body, r.params);
  track(name, res, expect);
  return res;
}

const exchangeReq = (p) => ({
  method: 'POST',
  url: `${URLS.authn}/authn.v1.TokenService/ExchangeToken`,
  body: '{}',
  params: { headers: headers(p.apiKey), tags: { name: 'exchange' }, timeout: '15s' },
});

// What a participant's own organization may return, per call.
const checks = {
  clusters: (p, b) => {
    const names = (b.clusters || []).map((c) => c.name).join(',');
    return check('own-clusters', names === p.cluster, `${p.email} sees clusters [${names}]`);
  },
  cluster: (p, b) => check('own-cluster', b.cluster?.id === p.clusterId, `${p.email} got cluster ${b.cluster?.id}`),
  pools: (p, b) => {
    const names = (b.nodePools || []).map((n) => n.name).join(',');
    return check('own-pools', names === 'pool', `${p.email} sees node pools [${names}]`);
  },
  projects: (p, b) => {
    const foreign = (b.projects || []).filter((x) => x.clusterId !== p.clusterId);
    return check('own-projects', foreign.length === 0, `${p.email} sees projects of cluster ${foreign[0]?.clusterId}`);
  },
  apikeys: (p, b) => {
    const ids = (b.apiKeys || []).map((k) => k.id);
    const foreign = ids.filter((id) => keyOwner[id] && keyOwner[id] !== p.email);
    return check('own-apikeys', ids.includes(p.apiKeyId) && foreign.length === 0, `${p.email} sees keys of ${foreign.map((id) => keyOwner[id]).join(',') || 'nobody, not even its own'}`);
  },
};

function invocation(p, kind, token) {
  switch (kind) {
    case 'clusters':
      return req(kind, 'ClusterService/ListClusters', token, p.orgId);
    case 'cluster':
      return req(kind, 'ClusterService/GetCluster', token, p.orgId, { clusterId: p.clusterId });
    case 'pools':
      return req(kind, 'ClusterService/ListNodePools', token, p.orgId, { clusterId: p.clusterId });
    case 'projects':
      return req(kind, 'ProjectService/ListProjects', token, p.orgId, { clusterId: p.clusterId });
    case 'apikeys':
      return req(kind, 'APIKeyService/ListAPIKeys', token, p.orgId);
    case 'kubectl':
      return {
        method: 'GET',
        url: `${URLS.k8s}/clusters/${p.clusterId}/api/v1/namespaces`,
        params: { headers: headers(token), tags: { name: kind }, timeout: '15s' },
      };
    default:
      throw new Error(`unknown call ${kind}`);
  }
}

// One functl invocation that needs a token of its own, for the writes below.
function exchange(p) {
  const r = exchangeReq(p);
  const res = http.request(r.method, r.url, r.body, r.params);
  return track('exchange', res) ? bodyOf(res).accessToken : null;
}

const kinds = ['clusters', 'cluster', 'pools', 'projects', 'apikeys', 'kubectl'];

function step(p) {
  const picks = Array.from({ length: 2 + Math.floor(Math.random() * 3) }, () => kinds[Math.floor(Math.random() * kinds.length)]);
  const tokens = http.batch(picks.map(() => exchangeReq(p))).map((res) => (track('exchange', res) ? bodyOf(res).accessToken : null));
  const sent = picks.map((kind, i) => [kind, tokens[i]]).filter(([, t]) => t);
  http.batch(sent.map(([kind, t]) => invocation(p, kind, t))).forEach((res, i) => {
    const kind = sent[i][0];
    if (track(kind, res) && checks[kind]) checks[kind](p, bodyOf(res));
  });

  const roll = Math.random();
  if (roll < 0.02) {
    const other = people[Math.floor(Math.random() * people.length)];
    const t = other.orgId !== p.orgId && exchange(p);
    if (t) send('deny', req('deny', 'ClusterService/ListClusters', t, other.orgId, {}, [403]), [403]);
  } else if (roll < 0.07) {
    writeCycle(p);
  }
}

function writeCycle(p) {
  const t = exchange(p);
  if (!t) return;
  const created = send('create', req('create', 'ProjectService/CreateProject', t, p.orgId, { name: `l${RUN}-${__VU}-${__ITER}`, clusterId: p.clusterId }));
  const pid = bodyOf(created).projectId;
  if (!pid) return;
  // Until the creator's grant has arrived, the delete is refused with 403.
  // A delete refused during a disruption is retried like a client would, so
  // projects_left counts only what fundament itself left behind; the grant
  // delay is measured only on a delete nothing else refused.
  const t0 = Date.now();
  let del;
  let disrupted = false;
  do {
    del = send('delete', req('delete', 'ProjectService/DeleteProject', t, p.orgId, { projectId: pid }, [200, 403]), [200, 403]);
    if (del.status === 403) sleep(0.05);
    else if (del.status !== 200) {
      disrupted = true;
      sleep(1);
    }
  } while ((del.status === 403 || del.status === 0 || del.status >= 500) && Date.now() - t0 < (disrupted ? 30000 : 15000));
  if (del.status === 200 && !disrupted) grantDelay.add(Date.now() - t0);
  if (!check('creator-grant', del.status !== 403, `${p.email} cannot delete ${pid} after 15 s`) || del.status !== 200) return;
  const after = send('projects', req('projects', 'ProjectService/ListProjects', t, p.orgId, { clusterId: p.clusterId }));
  if (after.status === 200) check('deleted-gone', !(bodyOf(after).projects || []).some((x) => x.id === pid), `${pid} still listed after delete`);
}

export function agent() {
  step(people[(__VU - 1) % people.length]);
  sleep(thinkMin + Math.random() * (thinkMax - thinkMin));
}

export function consoleTab() {
  const p = people[(__VU - 1) % people.length];
  const poll = send('poll', req('poll', 'ClusterService/ListClusters', p.jwt, p.orgId));
  if (poll.status === 200) checks.clusters(p, bodyOf(poll));
  if (__ITER % 4 === 0) {
    send('console', { method: 'GET', url: `${URLS.console}/`, params: { tags: { name: 'console' }, timeout: '15s' } });
    send('userinfo', {
      method: 'POST',
      url: `${URLS.authn}/authn.v1.AuthnService/GetUserInfo`,
      body: '{}',
      params: { headers: headers(p.jwt), tags: { name: 'userinfo' }, timeout: '15s' },
    });
  }
  sleep(5);
}
