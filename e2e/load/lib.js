// Shared by the load scenarios: targets, request helpers, content checks and
// the event log that `report.ts` reads after the run.
import http from 'k6/http';
import encoding from 'k6/encoding';
import { Counter } from 'k6/metrics';

const domain = __ENV.BASE_DOMAIN || 'fundament.localhost';
const port = __ENV.PORT || '8443';
export const URLS = {
  authn: `https://authn.${domain}:${port}`,
  org: `https://organization.${domain}:${port}`,
  console: `https://console.${domain}:${port}`,
  dex: `https://dex.${domain}:${port}`,
  k8s: `https://k8s-api.${domain}:${port}`,
};

// Tags names this run creates, so a rerun does not collide with what an
// aborted run left behind, and `just e2e load-verify` finds them.
export const RUN = __ENV.RUN || 'r0';

// EDGE, when set, sends every request to that address. Locally it is the k3d
// node's port 443: the k3d serverlb in front of 127.0.0.1:8443 runs nginx with
// 1024 file descriptors and drops connections from a few hundred clients.
export const baseOptions = {
  insecureSkipTLSVerify: __ENV.INSECURE === '1',
  hosts: __ENV.EDGE ? Object.fromEntries(Object.values(URLS).map((u) => [u.replace(/^https:\/\//, ''), `${__ENV.EDGE}:443`])) : {},
};

// A response with the expected status but the wrong content, such as another
// organization's cluster. content_checks counts the checks that ran, so a run
// whose requests all failed cannot pass on zero content failures.
export const contentFailures = new Counter('content_failures');
export const contentChecks = new Counter('content_checks');

// One JSON line per event for report.ts, written to the k6 log file:
//   fail  a request with an unexpected status
//   ok    a success, at most once per second per VU and name, and only in a
//         scenario that calls heartbeat(), so report.ts can bound how long a
//         call kept failing
//   check a content check that failed
const lastOk = {};
let heartbeats = false;
export function heartbeat() {
  heartbeats = true;
}

function event(kind, name, status, detail) {
  console.log(JSON.stringify({ t: Date.now(), kind, name, status, vu: __VU, detail: detail ? String(detail).slice(0, 200) : undefined }));
}

export function track(name, res, expect = [200]) {
  const ok = [].concat(expect).includes(res.status);
  if (!ok) event('fail', name, res.status, res.error || res.body);
  else if (heartbeats) {
    const now = Date.now();
    if (!lastOk[name] || now - lastOk[name] >= 1000) {
      lastOk[name] = now;
      event('ok', name, res.status);
    }
  }
  return ok;
}

export function check(name, ok, detail) {
  contentChecks.add(1, { name });
  if (!ok) {
    contentFailures.add(1, { name });
    event('check', name, 'check', detail);
  }
  return ok;
}

export function bodyOf(res) {
  try {
    return res.json();
  } catch (e) {
    return {};
  }
}

export function headers(token, org) {
  return {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(org ? { 'Fun-Organization': org } : {}),
  };
}

// expect: the status that counts as success, or a list of them.
export function call(name, method, url, body, hdrs, expect = 200, params = {}) {
  const ok = [].concat(expect);
  const res = http.request(method, url, body, {
    headers: hdrs,
    tags: { name },
    timeout: '15s',
    responseCallback: http.expectedStatuses(...ok),
    ...params,
  });
  track(name, res, ok);
  return res;
}

export function rpc(name, base, path, token, org, body = {}, expect = 200) {
  return call(name, 'POST', `${base}/${path}`, JSON.stringify(body), headers(token, org), expect);
}

// Password login, which Dex's static users support in the local deployment.
export function login(email) {
  const res = call('login', 'POST', `${URLS.authn}/login/password`, JSON.stringify({ email, password: 'password' }), headers());
  if (res.status !== 200) return null;
  const token = res.json('access_token');
  const claims = JSON.parse(encoding.b64decode(token.split('.')[1], 'rawurl', 's'));
  return { token, org: (claims.organization_ids || [])[0] };
}

// The requests the console makes for one page view after login.
export function pageView(s) {
  call('console', 'GET', `${URLS.console}/`, null, {});
  rpc('userinfo', URLS.authn, 'authn.v1.AuthnService/GetUserInfo', s.token, s.org);
  rpc('orgs', URLS.org, 'organization.v1.OrganizationService/ListOrganizations', s.token, s.org);
  const cl = rpc('clusters', URLS.org, 'organization.v1.ClusterService/ListClusters', s.token, s.org);
  const id = cl.status === 200 ? cl.json('clusters.0.id') : null;
  if (id) rpc('projects', URLS.org, 'organization.v1.ProjectService/ListProjects', s.token, s.org, { clusterId: id });
}

// p95 per call name; the names also make k6 print each call's latency.
export function latency(names, p95ms) {
  return Object.fromEntries(names.map((n) => [`http_req_duration{name:${n}}`, [`p(95)<${p95ms}`]]));
}
