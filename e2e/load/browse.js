// Participants in the browser, with the seeded acme-corp and globex users.
// The phases run one after the other; PHASES picks them (default storm,browse).
//
//   storm   a workshop starts: 5 logins and page views per second for 1 min
//   browse  open consoles: ramp to 150, hold, ramp to 300, hold. Each logs in
//           once, polls ListClusters every 5 s and views a page every fourth
//           poll
//   stress  page views without pauses, ramping to STRESS_MAX per second, to
//           find where the stack gives out. It has no thresholds: it is
//           expected to fail at the top.
import { sleep } from 'k6';
import { URLS, baseOptions, latency, login, pageView, rpc } from './lib.js';

const USERS = ['alice@acme-corp.com', 'bart@acme-corp.com', 'cindy@acme-corp.com', 'david@globex.com'];
const phases = (__ENV.PHASES || 'storm,browse').split(',');
const stressMax = Number(__ENV.STRESS_MAX || 400);

const all = {
  storm: { seconds: 60, executor: 'constant-arrival-rate', exec: 'storm', rate: 5, timeUnit: '1s', duration: '1m', preAllocatedVUs: 20, maxVUs: 200 },
  browse: {
    seconds: 750,
    executor: 'ramping-vus',
    exec: 'browse',
    startVUs: 0,
    gracefulRampDown: '10s',
    stages: [
      { duration: '2m', target: 150 },
      { duration: '5m', target: 150 },
      { duration: '2m', target: 300 },
      { duration: '3m', target: 300 },
      { duration: '30s', target: 0 },
    ],
  },
  stress: {
    seconds: 360,
    executor: 'ramping-arrival-rate',
    exec: 'stress',
    startRate: 20,
    timeUnit: '1s',
    preAllocatedVUs: 100,
    maxVUs: 1000,
    stages: [
      { duration: '5m', target: stressMax },
      { duration: '1m', target: stressMax },
    ],
  },
};

const scenarios = {};
const thresholds = {};
const p95 = Number(__ENV.P95_MS || 500);
let offset = 0;
for (const p of phases) {
  const { seconds, ...scenario } = all[p];
  scenarios[p] = { ...scenario, startTime: `${offset}s` };
  offset += seconds + 15;
  if (p === 'stress') continue;
  thresholds[`http_req_failed{scenario:${p}}`] = ['rate==0'];
  Object.assign(thresholds, latency(['login', 'console', 'userinfo', 'orgs', 'clusters', 'projects', 'poll'], p95, p));
}
export const options = { ...baseOptions, scenarios, thresholds };

export function storm() {
  const s = login(USERS[Math.floor(Math.random() * USERS.length)]);
  if (s) pageView(s);
}

let session = null;
export function browse() {
  if (!session) {
    session = login(USERS[__VU % USERS.length]);
    if (!session) {
      sleep(5);
      return;
    }
  }
  if (__ITER % 4 === 0) pageView(session);
  else rpc('poll', URLS.org, 'organization.v1.ClusterService/ListClusters', session.token, session.org);
  sleep(5);
}

let stressSession = null;
export function stress() {
  if (!stressSession) stressSession = login(USERS[__VU % USERS.length]);
  if (stressSession) pageView(stressSession);
}
