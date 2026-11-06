# Shared baseline: everything needed for a functional fundament + Gardener box,
# independent of the provider and free of secrets and per-instance data.
{ pkgs, lib, ... }:
let
  # The box login user. Keep in sync with BOX_USER in host/lib.sh.
  user = "fundament";
in
{
  imports = [
    ./network.nix
    ./instance-data.nix
    ./docker-disk.nix
    ./cache-disk.nix
    ./memlog.nix
  ];

  # Recipes that change the host (gardener-*) refuse to run without this marker.
  environment.etc."fundament-test-env".text = "";
  # State of the environment (CA, env files, logs, bundles), written by box/*.sh.
  systemd.tmpfiles.rules = [
    "d /var/lib/test-env 0755 ${user} users -"
    "d /var/lib/test-env/logs 0755 ${user} users -"
    "d /var/lib/test-env/bundles 0755 ${user} users -"
  ];

  # --- Access (public key only) ---------------------------------------------
  services.openssh = {
    enable = true;
    # Management SSH on 2022, NOT 22: Gardener's local kind bastion publishes
    # 172.18.255.22:22 on the host, which a host sshd on 0.0.0.0:22 would block.
    ports = [ 2022 ];
    settings.PasswordAuthentication = false;
    settings.KbdInteractiveAuthentication = false;
    settings.PermitRootLogin = "no";
  };

  users.users.${user} = {
    isNormalUser = true;
    extraGroups = [ "wheel" "docker" ];
    # Serial console fallback only; SSH stays key-only.
    initialPassword = "fundament";
  };
  # Dedicated test boxes: passwordless sudo so remote actions are non-interactive.
  security.sudo.wheelNeedsPassword = false;

  # --- Container / k8s dev toolchain --------------------------------------
  virtualisation.docker.enable = true;
  # Gardener's local shared registry is HTTP-only; the host's `docker push` needs it
  # marked insecure (per Gardener's getting_started_locally docs), else pushes go HTTPS.
  virtualisation.docker.daemon.settings.insecure-registries = [ "registry.local.gardener.cloud:5001" ];
  # The host runs systemd-resolved, so /etc/resolv.conf points at the 127.0.0.53 stub
  # — unreachable from containers. Docker's embedded DNS (127.0.0.11) still resolves
  # network-aliases (e.g. registry.local.gardener.cloud -> registry container) locally,
  # but needs a real upstream for external names (chart repos, public images), else
  # in-cluster DNS fails with "server misbehaving". Give it public forwarders.
  virtualisation.docker.daemon.settings.dns = [ "1.1.1.1" "8.8.8.8" ];
  programs.nix-ld.enable = true;       # let prebuilt dynamically-linked tools run on NixOS
  environment.sessionVariables = {
    # mise's prebuilt Node runs through nix-ld; building it from source takes an hour.
    MISE_NODE_COMPILE = "0";
    # Caches on the disk that outlives the VM (cache-disk.nix); box/lib.sh sets the same.
    MISE_DATA_DIR = "/var/lib/test-env-cache/mise";
    GOCACHE = "/var/lib/test-env-cache/go/build";
    GOMODCACHE = "/var/lib/test-env-cache/go/mod";
    SKAFFOLD_CACHE_FILE = "/var/lib/test-env-cache/skaffold/cache";
    BUN_INSTALL_CACHE_DIR = "/var/lib/test-env-cache/bun";
    GARDENER_REGISTRY_CACHE = "/var/lib/test-env-cache/gardener-registry";
  };
  boot.kernel.sysctl = {
    "fs.inotify.max_user_watches" = 1048576; # kind needs many watches/instances
    "fs.inotify.max_user_instances" = 8192;
  };

  # Gardener/fundament edit /etc/hosts at runtime (e.g. registry.local.gardener.cloud).
  # NixOS makes /etc/hosts a read-only /nix/store symlink by default; copy it instead
  # so runtime `sudo tee -a /etc/hosts` succeeds.
  environment.etc."hosts".mode = "0644";

  # registry.local.gardener.cloud is the host-side push target for Gardener's local
  # registry (HTTP, :5001). Other *.local.gardener.cloud names resolve through the
  # resolved drop-in in network.nix.
  networking.extraHosts = "127.0.0.1 registry.local.gardener.cloud";

  # Every recipe and interactive shell picks up the toolchain and the CA settings
  # that box/up.sh writes.
  programs.bash.interactiveShellInit = ''
    [ -r /var/lib/test-env/env ] && . /var/lib/test-env/env
    eval "$(mise activate bash)"
  '';

  environment.systemPackages = with pkgs; [
    git just jq rsync
    docker-compose
    kubectl kubernetes-helm
    k9s stern # looking around the platform cluster, the seed and the shoots
    kind k3d
    nssTools # certutil — mkcert (setup-certs) needs it to manage the NSS CA store
    yq-go openssl # Gardener's local setup needs these (it's Makefile-driven)
    mise # toolchain manager; reads the fundament repo's mise.toml (go, kubectl, skaffold, ...)
    gnumake # Gardener's local setup is Makefile-driven
    postgresql # psql for the cluster-worker integration tests and DB inspection
    chromium # the e2e suite drives it through BROWSER_PATH
    gcc python3
  ];

  # --- Nix ----------------------------------------------------------------
  nix.settings.experimental-features = [ "nix-command" "flakes" ];
  nix.settings.trusted-users = [ "root" user ];

  time.timeZone = "Europe/Amsterdam";

  # Set to the NixOS release you install FROM. Do not bump casually.
  system.stateVersion = "25.11";
}
