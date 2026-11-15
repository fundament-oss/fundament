// Functional probe, to run alone or while something is disrupted (a pod
// killed, a node drained, a rollout). Three scenarios side by side for
// DURATION:
//
//   reads    RATE per second: a fresh login, then a page view on a session
//            kept per VU, so reads are measured even while logins fail. Alice
//            sees exactly acme-cluster and acme-project; David (globex) is
//            refused acme-corp.
//   writes   one per 2 s: Alice creates a project and, once her own grant has
//            arrived, adds Bart as viewer. Bart must see the members within
//            15 s; Cindy, an organization viewer outside the project, must
//            see none. Alice deletes the project, and Bart must lose it
//            within 15 s.
//   browser  one per 2 s: the console's login through authn /login, Dex's
//            form and /callback, then GetUserInfo on the cookie alone.
//
// Without MAX_OUTAGE every request must succeed. With it, refused requests are
// judged by report.ts: a disruption may refuse requests for up to MAX_OUTAGE
// seconds per call. Either way every content check must hold, and enough of
// them must run.
import http from 'k6/http';
import { sleep } from 'k6';
import { Trend } from 'k6/metrics';
import { RUN, URLS, baseOptions, bodyOf, call, check, headers, heartbeat, latency, login, rpc, track } from './lib.js';

heartbeat();

const ACME = '019b4000-0000-7000-8000-000000000001';
const BART = '019b4000-1000-7000-8000-000000000002';
const duration = __ENV.DURATION || '5m';
const rate = Number(__ENV.RATE || 1);
const grantVisible = new Trend('grant_visible_ms', true);

// Half of the read cycles' checks (three per cycle), counted in reads alone, so
// a run where most read cycles end early on a failed request does not pass on
// the checks the writes and browser logins made.
const unit = /^(\d+)(s|m|h)$/.exec(duration);
if (!unit) throw new Error(`DURATION=${duration}: use one unit, such as 90s, 5m or 1h`);
const seconds = Number(unit[1]) * { s: 1, m: 60, h: 3600 }[unit[2]];
const minChecks = Math.floor((seconds * rate * 3) / 2);

export const options = {
  ...baseOptions,
  thresholds: {
    ...(__ENV.MAX_OUTAGE ? {} : { http_req_failed: ['rate==0'] }),
    content_failures: ['count==0'],
    [`content_checks{scenario:reads}`]: [`count>=${minChecks}`],
    dropped_iterations: ['count==0'],
    ...latency(['login', 'console', 'userinfo', 'clusters', 'projects', 'b:submit'], Number(__ENV.P95_MS || 500)),
  },
  scenarios: {
    reads: { executor: 'constant-arrival-rate', exec: 'reads', rate, timeUnit: '1s', duration, preAllocatedVUs: 5, maxVUs: 40 },
    writes: { executor: 'constant-arrival-rate', exec: 'writes', rate: 1, timeUnit: '2s', duration, preAllocatedVUs: 3, maxVUs: 20 },
    browser: { executor: 'constant-arrival-rate', exec: 'browser', rate: 1, timeUnit: '2s', duration, preAllocatedVUs: 3, maxVUs: 20 },
  },
};

const sessions = {};
function session(email) {
  if (!sessions[email]) sessions[email] = login(email);
  return sessions[email];
}

// A 401 means the kept session expired or was lost: log in again next time.
function keep(email, res) {
  if (res.status === 401) delete sessions[email];
  return res;
}

export function reads() {
  login('alice@acme-corp.com');
  const a = session('alice@acme-corp.com');
  if (!a) return;
  call('console', 'GET', `${URLS.console}/`, null, {});
  keep('alice@acme-corp.com', rpc('userinfo', URLS.authn, 'authn.v1.AuthnService/GetUserInfo', a.token, a.org));
  const cl = keep('alice@acme-corp.com', rpc('clusters', URLS.org, 'organization.v1.ClusterService/ListClusters', a.token, a.org));
  if (cl.status === 200) {
    const clusters = bodyOf(cl).clusters || [];
    const names = clusters.map((c) => c.name).sort().join(',');
    check('clusters', names === 'acme-cluster', `alice sees [${names}]`);
    if (clusters[0]) {
      const pr = rpc('projects', URLS.org, 'organization.v1.ProjectService/ListProjects', a.token, a.org, { clusterId: clusters[0].id });
      if (pr.status === 200) check('projects', (bodyOf(pr).projects || []).some((p) => p.name === 'acme-project'), 'acme-project missing');
    }
  }
  const d = session('david@globex.com');
  if (d) {
    // A 200 is another organization's data: a failed check, not a refusal.
    const r = keep('david@globex.com', rpc('deny', URLS.org, 'organization.v1.ClusterService/ListClusters', d.token, ACME, {}, [403, 401]));
    if (r.status === 200 || r.status === 403) check('deny', r.status === 403 && bodyOf(r).code === 'permission_denied', `david got ${r.status} on acme-corp`);
  }
}

// Polls as Bart until seen(response) holds or 15 s pass; returns whether it held.
function bartUntil(b, pid, seen, name) {
  const start = Date.now();
  while (Date.now() - start < 15000) {
    const r = http.post(`${URLS.org}/organization.v1.ProjectService/ListProjectMembers`, JSON.stringify({ projectId: pid }), {
      headers: headers(b.token, b.org),
      tags: { name },
      timeout: '10s',
      responseCallback: http.expectedStatuses(200, 403, 404),
    });
    track(name, r, [200, 403, 404]);
    if (seen(r)) {
      if (name === 'w:bart') grantVisible.add(Date.now() - start);
      return true;
    }
    sleep(0.1);
  }
  return false;
}

export function writes() {
  const a = session('alice@acme-corp.com');
  const b = session('bart@acme-corp.com');
  const c = session('cindy@acme-corp.com');
  if (!a || !b || !c) return;
  const cl = rpc('w:clusters', URLS.org, 'organization.v1.ClusterService/ListClusters', a.token, a.org);
  const cid = (bodyOf(cl).clusters || [])[0]?.id;
  if (!cid) return;
  const cr = rpc('w:create', URLS.org, 'organization.v1.ProjectService/CreateProject', a.token, a.org, { name: `p${RUN}-${__VU}-${__ITER}`, clusterId: cid });
  const pid = bodyOf(cr).projectId;
  if (!pid) return;

  // Alice's own grant on the new project comes through the authz outbox.
  const t0 = Date.now();
  let add;
  do {
    add = rpc('w:add', URLS.org, 'organization.v1.ProjectService/AddProjectMember', a.token, a.org, { projectId: pid, userId: BART, role: 'PROJECT_MEMBER_ROLE_VIEWER' }, [200, 403]);
    if (add.status === 403) sleep(0.05);
  } while (add.status === 403 && Date.now() - t0 < 15000);
  check('owner-grant', add.status !== 403, `alice cannot manage ${pid} after 15 s`);

  if (add.status === 200) {
    const members = (r) => (r.status === 200 ? (bodyOf(r).members || []).length : 0);
    check('viewer-grant', bartUntil(b, pid, (r) => members(r) >= 2, 'w:bart'), `bart cannot see ${pid} after 15 s`);
    const r = rpc('w:cindy', URLS.org, 'organization.v1.ProjectService/ListProjectMembers', c.token, c.org, { projectId: pid }, [200, 403]);
    if (r.status === 200 || r.status === 403) check('non-member', members(r) === 0, `cindy sees ${members(r)} members of ${pid}`);
  }

  // A delete refused during a disruption is retried for longer than
  // MAX_OUTAGE, so a project stays only when fundament itself left it.
  let del;
  const t1 = Date.now();
  do {
    del = rpc('w:delete', URLS.org, 'organization.v1.ProjectService/DeleteProject', a.token, a.org, { projectId: pid });
    if (del.status !== 200) sleep(1);
  } while (del.status !== 200 && Date.now() - t1 < 30000);
  if (del.status === 200 && add.status === 200) {
    const gone = (r) => r.status === 403 || r.status === 404 || (r.status === 200 && (bodyOf(r).members || []).length === 0);
    check('revoked', bartUntil(b, pid, gone, 'w:bart-gone'), `bart still sees ${pid} 15 s after its delete`);
  }
}

// The login follows redirects, and k6 counts each hop as a request of its own.
const redirects = http.expectedStatuses({ min: 200, max: 399 });

export function browser() {
  const jar = new http.CookieJar();
  const page = call('b:login', 'GET', `${URLS.authn}/login`, null, {}, 200, { jar, responseCallback: redirects });
  const m = /action="([^"]+)"/.exec(page.body || '');
  if (!m) {
    if (page.status === 200) check('b:form', false, 'no login form');
    return;
  }
  const done = call('b:submit', 'POST', `${URLS.dex}${m[1].replace(/&amp;/g, '&')}`, { login: 'alice@acme-corp.com', password: 'password' }, {}, 200, {
    jar,
    responseCallback: redirects,
  });
  if (done.status !== 200) return;
  check('b:landing', done.url.startsWith(URLS.console), `ended at ${done.url.slice(0, 120)}`);
  if (!check('b:cookie', Object.keys(jar.cookiesForURL(URLS.authn)).some((k) => k.startsWith('fundament_auth')), 'no session cookie')) return;
  call('b:userinfo', 'POST', `${URLS.authn}/authn.v1.AuthnService/GetUserInfo`, '{}', { 'Content-Type': 'application/json' }, 200, { jar });
}
