# In-place node updates on metal-stack (GEP-31 + MEP-3/MEP-7)

Status: proposal draft, 2026-08-08. Intended as the basis for an
upstream conversation with the metal-stack maintainers and for our own
prototyping. Research base: upstream code read at metal-hammer 83a0aa5,
metal-api afae26f, gardener master 63f040fe, MCM v0.62 line; line
references below are against those revisions and will drift.

## Part 1: design

### 1.1 Current situation

On metal-stack, "replace a node" means "erase the machine". The
lifecycle is: gardener/MCM deletes the machine -> the MCM driver calls
metal-api FreeMachine -> metal-bmc forces PXE + power cycle -> the
machine boots metal-hammer, which wipes ALL block devices before
entering Waiting (NVMe: `nvme format --ses=1` secure erase; others:
discard-mkfs with a full-disk dd fallback; metal-hammer cmd/root.go
L102-L142, wipe.go). The replacement machine is then allocated fresh,
and there is no same-machine affinity: the MCM driver passes the cluster
id as a placement tag, which metal-api uses for rack ANTI-affinity, then
picks randomly among the spread candidates (metal-api
datastore/machine.go FindWaitingMachine). The freed machine would not be
back in Waiting in time to be its own replacement anyway.

An allocated machine that simply reboots is untouched: boot order is
OS-disk-first, and metal-api additionally instructs the BMC to boot from
disk after installation. The wipe is only reached through PXE entry into
metal-hammer. Note the guard is the boot order, not the wipe logic: any
PXE entry without the reinstall flag (dead OS disk falling through to
PXE, reset boot order) also wipes everything.

Gardener replaces nodes routinely and autonomously: workers Kubernetes
minor updates, OS image version updates in the maintenance window,
kubelet config changes, CA and ServiceAccount key rotation, autoscaling,
hibernation wake with changed specs. Every one of these is today a
rolling replacement, i.e. a full wipe of every disk in the machine.

Consequence: node-local data cannot survive gardener day-2 operations.
That kills Rook/Ceph OSDs and csi-driver-lvm PVs on worker data disks.
This is not hypothetical: our proof-of-concept management cluster runs
rook-ceph on a raw NVMe of each worker, holding etcd backups and the
metal control plane database backups. Ceph survives a single roll only
through replication plus rebuild time, and MCM does not wait for
HEALTH_OK between replacements.

Both ecosystems already built the primitives to fix this; they were
never connected:

- Gardener [GEP-31](https://github.com/gardener/enhancements/tree/main/geps/0031-inplace-node-updates)
  "in-place node updates" (Alpha since v1.113, feature gate
  `InPlaceNodeUpdates` on gardener-apiserver, still Alpha as of
  2026-08). Per worker pool, `updateStrategy: AutoInPlaceUpdate |
  ManualInPlaceUpdate` makes k8s minor updates, kubelet config changes,
  CA/SA rotation, and OS image VERSION updates happen on the existing
  machine. MCM (>= v0.58) orchestrates cordon/drain and signals the node
  via the `InPlaceUpdate` node condition; gardener-node-agent executes
  the update; for the OS leg it runs a command the OS extension declares
  in `OperatingSystemConfig.status.inPlaceUpdates.osUpdate`. Crucially,
  the MCM DRIVER is never invoked: no CreateMachine/DeleteMachine
  happens, the Machine and Node objects survive. Still rolling by
  design: image NAME switch, machine type, volumes, CRI, worker
  providerConfig. Strategy switching rolling <-> in-place on an existing
  pool is forbidden (only Auto <-> ManualInPlace).
- metal-stack [MEP-3](https://metal-stack.io/community/mep-3-machine-re-installation/)
  "Machine Re-Installation to preserve local data" (Completed) plus
  [MEP-7](https://metal-stack.io/community/mep-7-configurable-filesystem-layout-for-machine-allocation/)
  filesystem layouts: metal-api `POST /v1/machine/{id}/reinstall` sets
  Allocation.Reinstall + new ImageID and power-cycles into metal-hammer,
  which SKIPS the global wipe and only zaps disks the filesystem layout
  marks `wipeonreinstall: true`; disks not listed in the FSL `disks:`
  section (the ceph disk) are untouched, existing LVM VGs referenced
  under `volumegroups:` (csi-lvm) survive. Requires the FSL to be
  reinstallable (>= 1 disk with wipeonreinstall true). The allocation,
  and with it machine identity (machine id, hostname, IPs), is
  preserved. MEP-3 explicitly names the missing gardener-side strategy
  as its open work item and cites Rook/Ceph as the motivating use case.
  The gardener half was never built.

The three metal-stack gardener components have zero in-place support
today: gardener-extension-provider-metal hard-codes RollingUpdate on
every MachineDeployment (pkg/controller/worker/machines.go), the MCM
driver only knows FreeMachine and never calls reinstall, and
os-metal-extension always returns nil for InPlaceUpdatesStatus. None of
this is a version blocker (the extension already vendors gardener 1.136
and MCM 0.61); it is missing code. provider-metal is absent from the
GEP-31 adoption checklist
([gardener#10219](https://github.com/gardener/gardener/issues/10219))
and no metal-stack issue tracks it.

Prior art elsewhere: all five adapted cloud providers (aws, gcp, azure,
alicloud, openstack) pair with the single adapted OS extension,
os-gardenlinux, whose `gardenlinux-update` does a transactional IN-NODE
update (stage new version from an OCI registry, reboot into it, roll
back on boot failure); local state survives, so the GEP-31 rollback
prerequisite is met natively. The only prior attempt at a
reinstall-shaped update is the Nov 2025 gardener hackathon prototype
(IronCore, not metal-stack) which extended the MCM driver interface with
UpdateMachine and was explicitly not pursued upstream over contract
concerns.

### 1.2 Design: implement GEP-31's OS leg with MEP-3 reinstall

Key insight: GEP-31 treats the osUpdate command as a black box that is
allowed to reboot the machine (gardenlinux-update reboots too), and the
MCM driver contract is untouched by in-place updates. So metal-stack can
implement the OS-update leg as "mark the machine for reinstall and
power-cycle into metal-hammer", triggered from the seed, without the
rejected UpdateMachine driver extension and without building a
transactional updater into metal-images. The new OS version is a clean
metal-hammer install of the stock image: zero drift, reuses the most
battle-tested path metal-stack has.

Two tiers, independently valuable:

- Tier 1 (kubelet-level in-place): adapt provider-metal the same way the
  cloud providers were adapted. Buys in-place k8s minor updates, kubelet
  config changes, and CA/SA rotation, which are the majority of
  gardener-initiated rolls. OS image updates still roll. No OS-side work
  at all.
- Tier 2 (OS updates via reinstall): a seed-side reinstall controller in
  provider-metal + a stub osUpdate command from os-metal-extension + a
  fresh bootstrap token carried via refreshed userdata + a small
  metal-api addition.

Runtime flow (tier 2): shoot bumps OS version on an in-place pool ->
gardenlet renders the new OSC -> MCM cordons/drains one node (within
maxUnavailable) and sets node condition InPlaceUpdate=ReadyForUpdate ->
gardener-node-agent runs the stub osUpdate command (blocks) -> the
reinstall controller sees ReadyForUpdate + pending OS version change,
mints a fresh bootstrap token, renders fresh userdata, calls metal-api
reinstall with the target image -> metal-bmc sets PXE + power cycles ->
metal-hammer skips the global wipe, zaps only wipeonreinstall disks,
installs the new image; allocation (machine id, hostname, IPs) is
preserved -> node boots, node-agent bootstraps fresh with the new token,
applies the current OSC as a first-time install, and because the Node
still carries ReadyForUpdate without a result label it patches
`node.machine.sapcloud.io/update-result=successful` -> MCM flips the
condition, moves the Machine to the new MachineSet, uncordons. The ceph
disk is never in the FSL disks list and is never touched.

Verified properties (code-read, not yet tested; the prototype phase
re-verifies live):

- The success signal is keyed ONLY off API-server state (the surviving
  Node's condition + absent result label), not off local files. A fully
  fresh node-agent completes the flow (gardener
  pkg/nodeagent/controller/operatingsystemconfig/reconciler.go, the gate
  around L865). No hang.
- Node identity is fully preserved: same Node object and UID (kubelet
  adopts an existing Node of its own name), same Machine object, same
  hostname/IPs from the preserved allocation. Kubelet re-issues its
  certs through the normal CSR flow, which does not depend on node
  novelty.
- A failed update parks the Machine in InPlaceUpdateFailed; MCM does NOT
  auto-replace machines in in-place phases, so a botched reinstall
  strands one drained node for an operator instead of triggering a
  surprise free+wipe.

Known gaps and risks, with mitigations:

1. Bootstrap credential (HARD blocker, drives the design). The token in
   the stored allocation userdata was minted once at machine creation
   with ~20 min expiry and MCM deletes the secret after node join. A
   literal re-bootstrap months later gets 401 and the update times out
   into InPlaceUpdateFailed. Fix: the reinstall controller mints a fresh
   bootstrap token per update and ships it in refreshed userdata
   (metal-api change, Part 2.4). Rejected alternative: preserving
   /var/lib/gardener-node-agent on disk, see 1.3.
2. False-success hole in gardener-node-agent. The "came back on the OLD
   version" check only runs when the pre-reboot on-disk marker survived;
   after a wipe the fresh-install branch reports success without
   re-checking the OS version. Needs a small provider-neutral upstream
   patch (Part 2.1). Until merged: the reinstall controller can verify
   the reported OS version out-of-band.
3. No rollback after the root disk is zapped. metal-hammer's abort path
   only kexecs back into the old OS if the primary disk was not yet
   wiped. This deviates from GEP-31's stated prerequisite (updater falls
   back on failure) and must be disclosed openly in the upstream
   proposal: bare metal trades rollback for zero on-node update tooling;
   the failure mode is one drained node in UpdateFailed.
4. Update duration vs MCM timeout. PXE + targeted zap (fast; the
   reinstall path does wipefs/sgdisk, not secure erase) + image install
   + reboot is realistically 5-10 min of NotReady. MCM's
   MachineInPlaceUpdateTimeout needs headroom; verify default and
   configurability during prototyping.
5. FSL semantics caveat (flagged code-reading, verify live): on
   reinstall, EVERY disk listed under FSL `disks:` gets wipefs + sgdisk
   repartition and its layout filesystems re-mkfs'd; wipeonreinstall
   only adds --zap-all. Preservation granularity is per DISK and means
   "not listed" (or VG-only). Layouts must keep data disks out of
   `disks:` and mark the OS disk wipeonreinstall true (also required for
   IsReinstallable).
6. Low risk, on the test list: the kubelet serving-cert CSR approver
   against a pre-existing Node (it validates hostname/machine
   correspondence, which is unchanged).

### 1.3 Considered and rejected alternatives

- Transactional in-node updater in metal-images (the gardenlinux shape):
  largest possible work item, a whole OS update mechanism (A/B scheme,
  distribution channel reachable from workers) for images designed to be
  written once by metal-hammer. Rejected on effort; also duplicates what
  reinstall already does well.
- Garden Linux as worker OS on metal-stack: would make the existing
  os-gardenlinux machinery work nearly out of the box (tier 2 shrinks to
  tier 1). Path of least upstream resistance but a fleet-wide OS change;
  worth naming when talking to the metal-stack maintainers, not this
  plan.
- MCM driver UpdateMachine extension: already prototyped and rejected
  upstream (contract concerns). This design deliberately routes around
  it with a seed-side controller behind the standard osUpdate hook.
- Separate state partition/disk for /var/lib/gardener-node-agent so
  credentials survive: a same-disk partition cannot survive (per-disk
  FSL granularity, the OS disk is repartitioned every install); a
  dedicated state DISK works today but is a hardware requirement, adds
  mount choreography, and keeps long-lived node credentials across
  reinstalls where the token approach re-mints short-lived ones.
  Rejected; preempt it in the proposal.

## Part 2: implementation plan

Ordered by repo. No code diffs here, but enough detail to scope PRs.

### 2.1 gardener/gardener (upstream, small robustness patch)

In pkg/nodeagent/controller/operatingsystemconfig/changes.go, the
no-last-applied-OSC branch (L95-L139 at 63f040fe) leaves all
InPlaceUpdates flags false, so a post-wipe boot on the WRONG OS version
still ends in patchNodeUpdateSuccessful (reconciler.go L865-L874).
Change: when `osc.Spec.InPlaceUpdates != nil` and the node carries the
in-place condition, also compute
`InPlaceUpdates.OperatingSystem = !IsOsVersionUpToDate(...)` (live
/etc/os-release vs desired, the same comparison the normal branch does
at L157-L162) so the version gate and the rollback/failed detection
(reconciler.go L820-L840) run for state-less updaters too. Small,
provider-neutral, justifiable on its own merits (any updater that loses
local state hits this). File as an issue first, referencing GEP-31's
bare-metal motivation.

### 2.2 gardener-extension-provider-metal (bulk of the work)

a) Worker controller (pkg/controller/worker/machines.go ~L246): replace
   the hard-coded RollingUpdateMachineDeploymentStrategyType with a
   mapping from `pool.UpdateStrategy` to MCM's InPlaceUpdate strategy
   (orchestrationType auto|manual from Auto/ManualInPlaceUpdate,
   maxSurge/maxUnavailable passed through). Template: what provider-aws
   did in
   [its adaptation PR](https://github.com/gardener/gardener-extension-provider-aws/pull/1276).
b) Worker-pool hash: adopt the split calculation (WorkerPoolHashV2) so
   in-place-updatable fields (kubelet version/config, OS version) are
   excluded from the hash that forces machine replacement but kept in
   the MachineClass hash.
c) Validation (admission/webhook): on pools with an in-place strategy,
   reject changes that cannot be applied in-place: machine type, volume
   type/size, CRI, machine image NAME, and worker providerConfig
   changes (MCM has no UpdateMachine and node-agent cannot apply
   providerConfig).
d) Worker status: populate `status.inPlaceUpdates.workerPoolToHashMap`
   so gardenlet tracks pending in-place updates.
e) NEW reinstall controller (tier 2, the actual GEP-31/MEP-3 tie):
   - Watch shoot Nodes (the extension already has shoot + seed clients
     and metal-api credentials per shoot namespace).
   - Trigger condition: node condition InPlaceUpdate=ReadyForUpdate AND
     the pool's desired OS version differs from the node's current one
     AND no reinstall already in flight for this machine. Drain is
     already done by MCM at this point.
   - Resolve the target metal image ID from the worker pool's machine
     image name+version via the existing cloudprofile image mapping.
   - Mint a fresh bootstrap token in the shoot (same mechanism MCM uses
     at machine creation, machine-controller-manager
     pkg/util/provider/machinecontroller/userdata.go: token secret in
     kube-system, expiry covering the update window), render fresh
     userdata from the machine's MachineClass with the token
     substituted.
   - Call metal-api `POST /v1/machine/{id}/reinstall` with the image ID
     and the fresh userdata (needs 2.4).
   - Bookkeeping: annotate the Machine/Node with the triggered image +
     timestamp to be idempotent across controller restarts; surface
     errors as events; do NOT retry into a power-cycle loop (cap
     attempts, leave the node for MCM's timeout to mark UpdateFailed).
   - Until 2.1 is merged upstream: after the node reports success,
     cross-check the node's reported OS version and emit a warning
     event on mismatch.
f) Vendoring: bump gardener/MCM deps as needed (already >= the required
   versions today; keep them there).

### 2.3 os-metal-extension

Actuator (pkg/controller/operatingsystemconfig/actuator.go) currently
returns nil InPlaceUpdatesStatus unconditionally. Change: when
`osc.Spec.InPlaceUpdates != nil`, deploy a small script via the OSC
files section (e.g. /opt/metal/os-update.sh) and return
`status.inPlaceUpdates.osUpdate = {command: that script, args: [target
os version]}`. Script semantics: it does NOT perform the update; the
seed-side controller does. It blocks (sleep loop) until the machine is
power-cycled out from under it, which is within the GEP contract (the
gardenlinux command also ends in a reboot; node-agent handles
resume-after-boot, and in our case the post-boot path is the
fresh-install branch). Optionally the script can first write a node
annotation as an explicit "node side ready" signal, but the
ReadyForUpdate condition already encodes drain-complete, so keep the
script dumb unless prototyping shows a race worth closing.

### 2.4 metal-api (small, backward compatible)

Add optional userdata to the reinstall request
(V1MachineReinstallRequest gains a userdata field; reinstallMachine in
cmd/metal-api/internal/service/machine-service.go ~L1904 sets
Allocation.UserData before publishing MachineReinstallCmd). Without it,
the fresh token must be smuggled by parsing the old token out of the
stored allocation userdata and recreating it under the same id, which is
workable but ugly; the one-field addition keeps the design clean.
Fallback if upstream declines: implement the token-recreation variant
entirely in the reinstall controller.

### 2.5 machine-controller-manager-provider-metal

No driver interface changes, ever, in this design. Verify the repo's
vendored MCM is >= v0.58 so the embedded machine controller carries the
in-place phases (InPlaceUpdating etc.) and rebuild; if it is older this
is a mechanical dep bump. (gardener-extension-provider-metal pins MCM
0.61.2; this repo's go.mod was not checked during research.)

### 2.6 Unchanged

metal-hammer, metal-bmc, metal-images: zero changes. The reinstall path
(skip global wipe, wipeonreinstall zap, abort-with-kexec before the
primary disk is wiped, boot-order restore), the MachineReinstallCmd BMC
handling (PXE + power cycle), and the images work as-is. Optional
follow-up to propose separately: make wipeonreinstall=false on a LISTED
disk actually skip wipefs/sgdisk/mkfs on reinstall, which is what the
MEP-7 field name suggests; not required by this plan.

### 2.7 Landscape configuration (ours, once the code exists)

- CloudProfile: `inPlaceUpdateConfig: {supported: true,
  minVersionForUpdate: ...}` on the machine image versions.
- Feature gate `InPlaceNodeUpdates` on gardener-apiserver (still Alpha;
  test on a non-production landscape first).
- FSLs: OS disk wipeonreinstall true, data disks never in `disks:`.
- Shoots: new worker pools with `updateStrategy: AutoInPlaceUpdate`
  (existing pools cannot be switched; one last roll to migrate).
- Extension version bumps; test landscape first.

### 2.8 Sequencing

- M0 prototype (no code): `metalctl machine reinstall` against a test
  worker carrying dummy data on an unlisted disk. Proves/measures: FSL
  caveat 1.2(5), reinstall duration vs timeout, allocation
  preservation, and (with a hand-minted token + hand-edited userdata)
  the fresh node-agent bootstrap + UpdateSuccessful flow.
- M1 tier 1 PRs to provider-metal (2.2 a-d). Standalone value: k8s and
  kubelet-level updates stop wiping nodes. This is the easy upstream
  conversation; provider-aws is the precedent.
- M2 gardener node-agent patch (2.1). Independent, small.
- M3 metal-api userdata-on-reinstall (2.4). Independent, small.
- M4 reinstall controller + os-metal stub (2.2e, 2.3). Depends on
  M1-M3. End-to-end test on a test landscape first.
- Throughout: open the design discussion with the metal-stack
  maintainers early, framed as "finishing MEP-3's open work item with
  GEP-31 having arrived since"; they wrote down this exact goal,
  including the Rook/Ceph use case.

### 2.9 Open questions

- Bootstrap token lifetime/ownership: who garbage-collects tokens the
  reinstall controller mints; expiry sizing vs the MCM in-place timeout.
- MachineInPlaceUpdateTimeout default value and per-landscape tuning.
- Exact image-version -> metal image ID mapping for the controller
  (worker pool providerConfig vs cloudprofile lookup).
- Whether gardener upstream wants the "state-less updater" case (2.1)
  covered by contract or documented as OS-extension responsibility.
- Storage-aware pacing: even in-place, one node is down 5-10 min per
  update, and MCM does not wait for Ceph health between nodes. Decide
  whether ManualInPlaceUpdate (operator paces, waits for HEALTH_OK) is
  the right default for storage-bearing pools vs AutoInPlaceUpdate with
  maxUnavailable=1.
