# Pull-through Docker Hub mirror with storage on the persistent volume, so box
# recreation doesn't re-download public images from the internet (morning speedup,
# design D8). Scope: docker.io pulls made by the HOST docker daemon (k3d/kind node
# images, build bases) — daemon `registry-mirrors` only covers Docker Hub, and
# pulls made *inside* k3d/kind nodes go through their own containerd, not this.
# Push targets (k3d registry :5111, gardener registry :5001) are rebuilt from
# local builds and deliberately not persisted.
{ ... }:
{
  virtualisation.oci-containers.backend = "docker";
  virtualisation.oci-containers.containers.registry-mirror = {
    image = "registry:2";
    ports = [ "127.0.0.1:5999:5000" ];
    # A root-owned top-level dotdir on the volume — deliberately NOT under
    # ~/.cache, so docker's root-owned bind source never breaks user-level
    # cache tooling (rm -rf ~/.cache etc.).
    volumes = [ "/home/fundament/.registry-mirror:/var/lib/registry" ];
    environment.REGISTRY_PROXY_REMOTEURL = "https://registry-1.docker.io";
  };
  # HARD-require the home mount (not just ordering): starting against the
  # root-disk path would let the later mount shadow the live cache — silently
  # caching to the disk that dies with the box. No volume → no mirror; the
  # daemon falls back to Docker Hub anyway.
  systemd.services.docker-registry-mirror = {
    after = [ "home-fundament.mount" ];
    unitConfig.RequiresMountsFor = [ "/home/fundament" ];
  };

  # The daemon falls back to Docker Hub when the mirror is unreachable — which
  # also resolves the bootstrap chicken-and-egg (pulling registry:2 itself).
  virtualisation.docker.daemon.settings.registry-mirrors = [ "http://localhost:5999" ];
}
