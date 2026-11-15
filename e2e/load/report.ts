/**
 * Reads what `just e2e load` collected for one run and fails it when a limit
 * is crossed. k6's own thresholds have already judged the requests; this adds
 * what k6 cannot see.
 *
 *   bun load/report.ts <dir> <scenario>
 *
 *   <scenario>-events.jsonl  fail, ok and check events from lib.js
 *   <scenario>-db.csv        the database samples taken during the run
 *   <scenario>-verify.txt    the end state, key=value per line
 *
 *   MAX_OUTAGE  seconds a call may keep failing, for disruption runs; unset,
 *               refused requests are left to k6's thresholds
 *   MAX_LAG     seconds the oldest unprocessed authz outbox row may reach
 *               (default 5, organization-api's CIRCUIT_BREAKER_THRESHOLD)
 *   MAX_CONN    share of max_connections the database may reach (default 0.9)
 */

import { existsSync, readFileSync } from 'node:fs';

const [dir, scenario] = process.argv.slice(2);

// A limit that is not a number would make every comparison false and pass.
function limit(name: string, fallback?: number): number | undefined {
  const raw = process.env[name];
  if (raw === undefined || raw === '') return fallback;
  const value = Number(raw);
  if (!Number.isFinite(value) || value < 0) throw new Error(`${name}=${raw} is not a number`);
  return value;
}
const maxOutage = limit('MAX_OUTAGE');
const maxLag = limit('MAX_LAG', 5)!;
const maxConn = limit('MAX_CONN', 0.9)!;
const breaches: string[] = [];

type Event = { t: number; kind: 'fail' | 'ok' | 'check'; name: string; status: number | string; detail?: string };

function lines(file: string): string[] {
  const path = `${dir}/${file}`;
  return existsSync(path) ? readFileSync(path, 'utf8').split('\n').filter(Boolean) : [];
}

// k6 writes other log lines to the same file; only the JSON events count.
const events: Event[] = lines(`${scenario}-events.jsonl`).flatMap((l) => {
  try {
    const e = JSON.parse(l);
    return e && e.kind ? [e] : [];
  } catch {
    return [];
  }
});

// Failures per call and status, with one example each.
const failures = new Map<string, { name: string; status: Event['status']; count: number; detail?: string }>();
for (const e of events.filter((e) => e.kind !== 'ok')) {
  const key = JSON.stringify([e.name, e.status]);
  const f = failures.get(key) ?? { name: e.name, status: e.status, count: 0, detail: e.detail };
  f.count++;
  failures.set(key, f);
}
console.log('\n== failures by call and status');
if (failures.size === 0) console.log('none');
for (const f of [...failures.values()].sort((a, b) => b.count - a.count)) {
  console.log(`${String(f.count).padStart(8)}  ${f.name}  ${f.status}  ${(f.detail ?? '').replace(/\s+/g, ' ').slice(0, 100)}`);
}

// Longest outage per call: from a failure to the next success of that call
// by any VU. A scenario that calls heartbeat() logs successes once a second
// per VU, so the bound is about a second coarse.
if (maxOutage !== undefined) {
  if (!events.some((e) => e.kind === 'ok')) breaches.push('no successful request was logged, so no outage can be measured');
  const byName = new Map<string, Event[]>();
  for (const e of events.filter((e) => e.kind === 'fail' || e.kind === 'ok')) {
    byName.set(e.name, [...(byName.get(e.name) ?? []), e]);
  }
  console.log(`\n== longest outage per call (limit ${maxOutage} s)`);
  for (const [name, list] of byName) {
    list.sort((a, b) => a.t - b.t);
    let worst = 0;
    let since: number | undefined;
    for (const e of list) {
      if (e.kind === 'fail') since ??= e.t;
      else if (since !== undefined) {
        worst = Math.max(worst, e.t - since);
        since = undefined;
      }
    }
    if (since !== undefined) {
      worst = Infinity;
    }
    if (worst === 0) continue;
    const label = worst === Infinity ? 'never recovered' : `${(worst / 1000).toFixed(1)} s`;
    console.log(`${name.padEnd(14)} ${label}`);
    if (worst > maxOutage * 1000) breaches.push(`${name} failed for ${label}, limit ${maxOutage} s`);
  }
}

// The end state and the run's start and end, key=value per line.
const verify = Object.fromEntries(lines(`${scenario}-verify.txt`).map((l) => l.split('=', 2)));

// Database peaks from the samples. Without them the limits cannot be checked.
const samples = lines(`${scenario}-db.csv`).map((l) => l.split(','));
if (samples.length === 0) breaches.push(`no database samples in ${scenario}-db.csv`);
else {
  // A failed sample query leaves a gap, most likely while Postgres is full,
  // also before the first sample or after the last one.
  const times = samples.map((s) => Number(s[0])).sort((a, b) => a - b);
  const bounds = [Number(verify.run_start) || times[0], ...times, Number(verify.run_end) || times[times.length - 1]];
  const gap = Math.max(0, ...bounds.slice(1).map((t, i) => t - bounds[i]));
  if (gap > 10) breaches.push(`no database sample for ${gap} s`);
  const num = (i: number) => samples.map((s) => Number(s[i])).filter((n) => !Number.isNaN(n));
  const peak = (i: number) => Math.max(...num(i));
  const conns = peak(1);
  const limit = peak(2);
  const lag = peak(4);
  console.log(`\n== database (${samples.length} samples)`);
  console.log(`connections   peak ${conns} of ${limit}`);
  console.log(`outbox        peak ${peak(3)} unprocessed, oldest ${lag.toFixed(1)} s (limit ${maxLag} s)`);
  const busiest = samples.reduce((a, s) => (Number(s[1]) > Number(a[1]) ? s : a));
  console.log(`at the peak   ${busiest[6] ?? ''}`);
  if (conns > limit * maxConn) breaches.push(`database connections peaked at ${conns} of ${limit}`);
  if (lag > maxLag) breaches.push(`oldest unprocessed authz outbox row reached ${lag.toFixed(1)} s`);
}

// End state after the run has settled; each field must be there and a number.
console.log('\n== end state');
for (const [k, v] of Object.entries(verify)) console.log(`${k.padEnd(28)} ${v}`);
for (const k of ['run_start', 'run_end', 'outbox_unprocessed', 'outbox_failed', 'projects_left']) {
  if (verify[k] === undefined || verify[k] === '' || !Number.isFinite(Number(verify[k]))) breaches.push(`end state ${k} is missing or not a number`);
}
if (Number(verify.outbox_unprocessed) > 0) breaches.push(`${verify.outbox_unprocessed} authz outbox rows still unprocessed`);
if (Number(verify.outbox_failed) > 0) breaches.push(`${verify.outbox_failed} authz outbox rows failed during the run`);
if (Number(verify.projects_left) > 0) breaches.push(`${verify.projects_left} projects of this run were not deleted`);

if (breaches.length > 0) {
  console.log(`\n== FAILED\n${breaches.map((b) => `- ${b}`).join('\n')}`);
  process.exit(1);
}
console.log('\n== passed');
