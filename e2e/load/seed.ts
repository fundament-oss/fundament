/**
 * Participants for agents.js on the local deployment: organizations load-NN,
 * each with PER admins (load-NN-K@fieldlab.test), one cluster with a node pool,
 * and an API key per participant. Run it through `just e2e load-seed` and
 * `just e2e load-clean`, which set the environment below.
 *
 *   bun load/seed.ts seed <teams> <per-team> <out.json>
 *   bun load/seed.ts clean
 *
 *   FUNOPS                  path of a built funops binary
 *   FUNOPS_DB_HOST          host:port of the database funops connects to
 *   FUNOPS_DB_PASSWORD      fun_operator's password
 *   JWT_SECRET              authn-api's signing secret: participants have no
 *                           password login, so the seed signs their sessions
 *   ORGANIZATION_API_URL    defaults to the local deployment
 *
 * Seeding again reuses the organizations, members and clusters that exist,
 * and gives every participant a new API key, since a key is shown only once.
 */

import { execFileSync } from 'node:child_process';
import { createHmac } from 'node:crypto';
import { writeFileSync } from 'node:fs';

const organizationApiUrl = process.env.ORGANIZATION_API_URL ?? 'https://organization.fundament.localhost:8443';
const prefix = 'load-';

function required(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is not set; run this through just e2e load-seed`);
  return value;
}

const funopsEnv = {
  ...process.env,
  DATABASE_URL: `postgresql://fun_operator:${encodeURIComponent(required('FUNOPS_DB_PASSWORD'))}@${required('FUNOPS_DB_HOST')}/fundament`,
};

function funops(...args: string[]): any {
  return JSON.parse(execFileSync(required('FUNOPS'), ['-o', 'json', ...args], { encoding: 'utf8', env: funopsEnv, stdio: ['ignore', 'pipe', 'pipe'] }) || 'null');
}

const b64 = (data: string | Buffer) => Buffer.from(data).toString('base64url');

// The claims authn-api puts in a session token, valid for a day.
function sessionToken(userId: string, organizationId: string, name: string): string {
  const now = Math.floor(Date.now() / 1000);
  const head = b64(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const body = b64(
    JSON.stringify({ iss: 'fundament-authn-api', sub: userId, aud: ['fundament-user'], iat: now, exp: now + 86400, organization_ids: [organizationId], name }),
  );
  const sig = b64(createHmac('sha256', required('JWT_SECRET')).update(`${head}.${body}`).digest());
  return `${head}.${body}.${sig}`;
}

// A new organization's grants reach OpenFGA through the authz outbox, so a
// call may be refused (403) or unavailable (5xx) for a moment; other errors
// are final.
async function rpc(path: string, token: string, organizationId: string, body: object): Promise<any> {
  const deadline = Date.now() + 60_000;
  for (;;) {
    let status = 0;
    let text = '';
    try {
      const res = await fetch(`${organizationApiUrl}/organization.v1.${path}`, {
        method: 'POST',
        headers: { authorization: `Bearer ${token}`, 'fun-organization': organizationId, 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
      status = res.status;
      text = await res.text();
      if (res.ok) return JSON.parse(text);
    } catch (err) {
      text = String(err);
    }
    const transient = status === 0 || status === 403 || status >= 500;
    if (!transient || Date.now() > deadline) throw new Error(`${path}: ${status} ${text.slice(0, 300)}`);
    await new Promise((r) => setTimeout(r, 500));
  }
}

type Participant = { email: string; orgId: string; jwt: string; cluster: string; clusterId: string; apiKeyId: string; apiKey: string };

async function seed(teams: number, perTeam: number, out: string) {
  const existing = new Map<string, string>((funops('organization', 'list') ?? []).map((o: { name: string; id: string }) => [o.name, o.id]));
  const people: Participant[] = [];
  let offer: { region: string; machineType: string; version: string } | undefined;

  for (let t = 1; t <= teams; t++) {
    const org = `${prefix}${String(t).padStart(2, '0')}`;
    let organizationId = existing.get(org);
    if (!organizationId) {
      organizationId = funops('organization', 'create', org).id as string;
      funops('organization', 'quota', 'set-clusters', org, '5');
    }
    const members = new Set((funops('organization', 'member', 'list', org) ?? []).map((m: { email: string }) => m.email));
    for (let k = 1; k <= perTeam; k++) {
      const email = `${org}-${k}@fieldlab.test`;
      if (!members.has(email)) funops('organization', 'member', 'add', org, email, '--permission', 'admin', '--create-user');
    }
    const team = (funops('organization', 'member', 'list', org) as { email: string; user_id: string }[])
      .filter((m) => m.email.startsWith(`${org}-`))
      .map((m) => ({ email: m.email, jwt: sessionToken(m.user_id, organizationId!, m.email) }));
    const first = team[0].jwt;

    // The first organization learns what the platform offers.
    if (!offer) {
      const { regions } = await rpc('ClusterService/ListRegions', first, organizationId, {});
      const region = regions[0];
      const versions = [...region.kubernetesVersions].sort((a: string, b: string) => a.localeCompare(b, undefined, { numeric: true }));
      offer = { region: region.name, machineType: region.machineTypes[0].name, version: versions[versions.length - 1] };
    }
    funops('organization', 'quota', 'set-nodes', org, offer.region, offer.machineType, '100');

    const cluster = `${org}-c`;
    const { clusters } = await rpc('ClusterService/ListClusters', first, organizationId, {});
    let clusterId: string | undefined = (clusters ?? []).find((c: { name: string }) => c.name === cluster)?.id;
    if (!clusterId) {
      clusterId = (await rpc('ClusterService/CreateCluster', first, organizationId, { name: cluster, region: offer.region, kubernetesVersion: offer.version })).clusterId;
      await rpc('ClusterService/CreateNodePool', first, organizationId, { clusterId, name: 'pool', machineType: offer.machineType, autoscaleMin: 1, autoscaleMax: 3 });
    }

    for (const m of team) {
      const key = await rpc('APIKeyService/CreateAPIKey', m.jwt, organizationId, { name: `agent-${Date.now().toString(36)}`, expiresIn: '24h' });
      people.push({ email: m.email, orgId: organizationId, jwt: m.jwt, cluster, clusterId: clusterId!, apiKeyId: key.id, apiKey: key.token });
    }
    console.log(`${org}: ${team.length} participants, cluster ${clusterId}`);
  }
  writeFileSync(out, JSON.stringify(people));
  console.log(`${people.length} participants in ${out}; their API keys and sessions expire in 24 h`);
}

function clean() {
  for (const o of funops('organization', 'list') ?? []) {
    if (!o.name.startsWith(prefix)) continue;
    funops('organization', 'delete', o.name);
    console.log(`deleted ${o.name}`);
  }
}

const [mode, ...args] = process.argv.slice(2);
if (mode === 'seed') await seed(Number(args[0]), Number(args[1]), args[2]);
else if (mode === 'clean') clean();
else throw new Error('usage: seed.ts seed <teams> <per-team> <out.json> | seed.ts clean');
