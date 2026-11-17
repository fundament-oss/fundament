// Participants with AI agents, from `just e2e load-seed`. One VU per
// participant, so no two VUs share a participant's key and rows:
//
//   agent     the first AGENTS VUs. An agent step is 2–4 CLI invocations in
//             parallel. Each invocation exchanges the API key and then makes
//             one call, as functl and the kubectl exec plugin do: lists and
//             gets, or kubectl through kube-api-proxy. 5 % of the steps create
//             a project, wait until the creator may delete it (the grant goes
//             through the authz outbox), delete it and check it is gone. 2 %
//             ask for another team's organization and cluster, which must be
//             refused. A pause of THINK_MIN–THINK_MAX s follows; an LLM turn
//             usually takes longer, so the default errs towards more load.
//   console   the first CONSOLES VUs (default: all). An open console polls
//             ListClusters every 5 s, as the dashboard does while a cluster is
//             changing, with a page view every fourth poll. A VU that is also
//             an agent polls between its steps.
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
import { RUN, URLS, baseOptions, bodyOf, check, failRate, headers, latency, request, rpcRequest, send, setting, track } from './lib.js';

const people = new SharedArray('people', () => JSON.parse(open(__ENV.PEOPLE)));
const duration = __ENV.DURATION || '10m';
const ramp = __ENV.RAMP || '2m';
const agents = setting('AGENTS', people.length);
const consoles = setting('CONSOLES', people.length);
const thinkMin = setting('THINK_MIN', 0.5);
const thinkMax = setting('THINK_MAX', 3);
const vus = Math.max(agents, consoles);
if (vus > people.length) {
  throw new Error(`${vus} VUs for ${people.length} participants: seed more (just e2e load-seed <teams> <per-team>)`);
}
if (vus === 0) throw new Error('AGENTS and CONSOLES are both 0: no VU would run');
if (thinkMin > thinkMax) throw new Error(`THINK_MIN=${thinkMin} is above THINK_MAX=${thinkMax}`);

// Owner of every seeded API key, to spot a key of someone else in a list.
const keyOwner = Object.fromEntries(people.map((p) => [p.apiKeyId, p.email]));

// Time from creating a project until its creator may delete it.
const grantDelay = new Trend('grant_delay_ms', true);

const calls = ['exchange', 'clusters', 'cluster', 'pools', 'projects', 'apikeys', 'kubectl', 'create', 'delete', 'deny', 'deny-projects', 'poll', 'userinfo', 'console'];
export const options = {
  ...baseOptions,
  noConnectionReuse: __ENV.REUSE !== '1',
  // A single scenario numbers its VUs 1..vus; across scenarios __VU counts
  // the whole test, and two VUs could map to one participant.
  scenarios: {
    participants: {
      executor: 'ramping-vus',
      exec: 'participant',
      startVUs: 0,
      gracefulRampDown: '5s',
      stages: [
        { duration: ramp, target: vus },
        { duration, target: vus },
      ],
    },
  },
  thresholds: {
    http_req_failed: [failRate],
    content_failures: ['count==0'],
    content_checks: ['count>0'],
    grant_delay_ms: [`p(95)<${setting('GRANT_P95_MS', 500)}`, `p(99)<${setting('GRANT_P99_MS', 2000)}`],
    ...latency(calls),
    // A refused ListProjects waits out organization-api's authz retry budget
    // for reads before it answers, so P95_MS does not apply.
    'http_req_duration{name:deny-projects}': ['max>=0'],
  },
};

const org = (name, path, token, orgId, body = {}, expect = 200) => rpcRequest(name, URLS.org, `organization.v1.${path}`, token, orgId, body, expect);
const exchangeReq = (p) => request('exchange', 'POST', `${URLS.authn}/authn.v1.TokenService/ExchangeToken`, '{}', headers(p.apiKey));

// Projects this run creates are named l<run>-<vu>-<iteration>; the VU names
// the participant, and with it the organization the project belongs to.
const runProject = new RegExp(`^l${RUN}-(\\d+)-`);
function foreignProject(p, x) {
  if (x.clusterId !== p.clusterId) return true;
  const m = runProject.exec(x.name || '');
  return m !== null && people[Number(m[1]) - 1]?.orgId !== p.orgId;
}

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
    const foreign = (b.projects || []).filter((x) => foreignProject(p, x));
    return check('own-projects', foreign.length === 0, `${p.email} sees project ${foreign[0]?.name} of cluster ${foreign[0]?.clusterId}`);
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
      return org(kind, 'ClusterService/ListClusters', token, p.orgId);
    case 'cluster':
      return org(kind, 'ClusterService/GetCluster', token, p.orgId, { clusterId: p.clusterId });
    case 'pools':
      return org(kind, 'ClusterService/ListNodePools', token, p.orgId, { clusterId: p.clusterId });
    case 'projects':
      return org(kind, 'ProjectService/ListProjects', token, p.orgId, { clusterId: p.clusterId });
    case 'apikeys':
      return org(kind, 'APIKeyService/ListAPIKeys', token, p.orgId);
    case 'kubectl':
      return request(kind, 'GET', `${URLS.k8s}/clusters/${p.clusterId}/api/v1/namespaces`, null, headers(token));
    default:
      throw new Error(`unknown call ${kind}`);
  }
}

// One functl invocation that needs a token of its own, for the writes below.
function exchange(p) {
  const res = send('exchange', exchangeReq(p));
  return res.status === 200 ? bodyOf(res).accessToken : null;
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
    if (t) {
      send('deny', org('deny', 'ClusterService/ListClusters', t, other.orgId, {}, 403), 403);
      // Another team's cluster, asked for under the participant's own organization.
      send('deny-projects', org('deny-projects', 'ProjectService/ListProjects', t, p.orgId, { clusterId: other.clusterId }, 403), 403);
    }
  } else if (roll < 0.07) {
    writeCycle(p);
  }
}

function writeCycle(p) {
  const t = exchange(p);
  if (!t) return;
  const created = send('create', org('create', 'ProjectService/CreateProject', t, p.orgId, { name: `l${RUN}-${__VU}-${__ITER}`, clusterId: p.clusterId }));
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
    del = send('delete', org('delete', 'ProjectService/DeleteProject', t, p.orgId, { projectId: pid }, [200, 403]), [200, 403]);
    if (del.status === 403) sleep(0.05);
    else if (del.status !== 200) {
      disrupted = true;
      sleep(1);
    }
  } while ((del.status === 403 || del.status === 0 || del.status >= 500) && Date.now() - t0 < (disrupted ? 30000 : 15000));
  if (del.status === 200 && !disrupted) grantDelay.add(Date.now() - t0);
  if (!check('creator-grant', del.status !== 403, `${p.email} cannot delete ${pid} after 15 s`) || del.status !== 200) return;
  const after = send('projects', org('projects', 'ProjectService/ListProjects', t, p.orgId, { clusterId: p.clusterId }));
  if (after.status === 200) check('deleted-gone', !(bodyOf(after).projects || []).some((x) => x.id === pid), `${pid} still listed after delete`);
}

// Per VU: when its console polls next, and how often it has polled.
let nextPoll = 0;
let polls = 0;

function consolePoll(p) {
  const poll = send('poll', org('poll', 'ClusterService/ListClusters', p.jwt, p.orgId));
  if (poll.status === 200) checks.clusters(p, bodyOf(poll));
  if (polls++ % 4 === 0) {
    send('console', request('console', 'GET', `${URLS.console}/`, null, {}));
    send('userinfo', request('userinfo', 'POST', `${URLS.authn}/authn.v1.AuthnService/GetUserInfo`, '{}', headers(p.jwt)));
  }
}

// Sleeps for the given seconds, polling the console whenever it is due.
function pause(p, tab, seconds) {
  const end = Date.now() + seconds * 1000;
  while (Date.now() < end) {
    if (tab && Date.now() >= nextPoll) {
      consolePoll(p);
      nextPoll = Date.now() + 5000;
    }
    const wake = tab ? Math.min(end, nextPoll) : end;
    if (wake > Date.now()) sleep((wake - Date.now()) / 1000);
  }
}

export function participant() {
  const p = people[__VU - 1];
  const agent = __VU <= agents;
  const tab = __VU <= consoles;
  if (agent) step(p);
  pause(p, tab, agent ? thinkMin + Math.random() * (thinkMax - thinkMin) : 5);
}
