# Known Issues

## Local Gardener: gardenlet crashes due to wrong seed node CIDR

**Symptom:** After `just cluster-worker gardener-up`, the gardenlet pod is in `CrashLoopBackOff` with:

```
panic: failed during bootstrapping: incorrect node network specified in seed configuration
(cluster node="172.30.0.2" vs. config="172.18.0.0/24")
```

The `local` Seed reports `GardenletReady=Unknown` ("Gardenlet stopped posting status updates") and any Shoots stay at `Create Pending (0%)`.

**Cause:** `dev-setup/gardenlet/base/gardenlet.yaml` (in the cloned upstream Gardener repo at `.dev/gardener/`) hard-codes the seed's `spec.networks.nodes` to `172.18.0.0/24` — the default `kind` Docker network. `just cluster-worker gardener-connect` attaches the kind container to the `k3d-fundament` network (`172.30.0.0/16`), so the container ends up dual-homed. Which of the two IPs kubelet advertises as the node's `InternalIP` is non-deterministic across runs (depends on interface enumeration / restart order). Gardenlet's bootstrap heuristic check refuses to start whenever the advertised IP falls outside the configured CIDR.

**Local workaround (already applied):** In `.dev/gardener/dev-setup/gardenlet/base/gardenlet.yaml`, set a CIDR that covers both Docker networks:

```yaml
seedConfig:
  spec:
    networks:
      nodes: 172.16.0.0/12  # was: 172.18.0.0/24 — covers both 172.18.x (kind) and 172.30.x (k3d-fundament)
```

After the change, run `just cluster-worker gardener-delete && just cluster-worker gardener-up` for it to take effect. **Do not try to patch a running cluster** — both the `Gardenlet` CR (`spec.config.seedConfig.spec.networks.nodes`) and the `Seed` object (`spec.networks.nodes`) are immutable once created. Live-patching the rendered ConfigMap also doesn't work: the gardener-operator reconciles it back from the `Gardenlet` CR.

**Caveat:** The workaround lives inside the `.dev/gardener/` clone of upstream Gardener and is **lost on any re-clone** (e.g., version bump in `cluster-worker/mod.just`, or `rm -rf .dev/gardener`).

**TODO:** Move the override into a fundament-side overlay so it survives a re-clone, or upstream a fix in Gardener that makes `dev-setup/gardenlet/base/gardenlet.yaml` work with non-default Docker networks (e.g., parameterize the node CIDR or detect it from the kind node).
