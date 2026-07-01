# Shared baseline: everything needed for a functional fundament + Gardener box,
# independent of hardware/cloud and free of secrets.
{ pkgs, lib, ... }:
let
  # The box login user — single source of truth. Keep in sync with BOX_USER in hetzner.sh.
  user = "thom";
in
{
  # --- Access (public key only — not secret) ------------------------------
  services.openssh = {
    enable = true;
    # Management SSH on 2022, NOT 22: Gardener's local kind bastion publishes
    # 172.18.255.22:22 on the host, which a host sshd on 0.0.0.0:22 would block.
    ports = [ 2022 ];
    settings.PasswordAuthentication = false;
  };

  users.users.${user} = {
    isNormalUser = true;
    extraGroups = [ "wheel" "docker" ];
    # Console fallback so the box is never locked out if Wi-Fi/SSH is down.
    # (SSH stays key-only; this is just for the physical login prompt.) Change as desired.
    initialPassword = "fundament";
    # Inbound admin key (your MacBook). Public — safe to commit.
    openssh.authorizedKeys.keys = [
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDeBNB2x9gB2rz29FX/QaYyoNU2SZv0iTUQuhs1/ePA4YKeZ169ffxmBF0aohELepVZgvSdrH6JoXNpy2dw/8pNnZ3ZtZ8NKyzsk4a02Hz19dupmTEBa31jl4p4vRNuraxmK08yzKSDxj3/JzEE6QLrFW3fR0WPn1JbGCT0uJvEHmHr34c6v36Y+jWKkr266Ls6sUSMfTaw6cRRkS03kfX7s2O/+7rCAMdUNX3PxGxJWyFSQoGtJsLsjE3dS6vRxNDo/sMvADSPwbMrvi6NXUBqkKezkbllcMeP+agmHXKNf8ec0DHGx2F/c/geR30tuzV9g5HZ+a+/syAz0e7gG4SkgxwQn4d7KmBCPFz3XvyS1qicAwx1B0PzaUSGZdCvMCjB23QA6nASkZ2J84BMoBtF2vUbJrZG3iBzJQifbLqUhmGE7aBm8j55BrZ44z4DeOAbKkVz+WpOscNbCzNC3fFZcf8dz7YvmwEPAuwYr6Tqz0J4cNTBkM6qwdTnxxn05r8= thom@Thoms-MacBook-Pro.local"
    ];
  };
  # Dedicated test boxes: passwordless sudo so remote actions are non-interactive.
  security.sudo.wheelNeedsPassword = false;

  # --- Container / k8s dev toolchain --------------------------------------
  virtualisation.docker.enable = true; # overlay2 default; runs on the scratch volume
  # Gardener's local shared registry is HTTP-only; the host's `docker push` needs it
  # marked insecure (per Gardener's getting_started_locally docs), else pushes go HTTPS.
  virtualisation.docker.daemon.settings.insecure-registries = [ "registry.local.gardener.cloud:5001" ];
  # The host runs systemd-resolved, so /etc/resolv.conf points at the 127.0.0.53 stub
  # — unreachable from containers. Docker's embedded DNS (127.0.0.11) still resolves
  # network-aliases (e.g. registry.local.gardener.cloud -> registry container) locally,
  # but needs a real upstream for external names (chart repos, public images), else
  # in-cluster DNS fails with "server misbehaving". Give it public forwarders.
  virtualisation.docker.daemon.settings.dns = [ "1.1.1.1" "8.8.8.8" ];
  # Gardener's local kind nodes reach host-published services on the 172.18.255.x
  # loopback IPs (bind9 DNS, ingress) through the docker/kind bridges. The NixOS
  # firewall drops that bridge->host INPUT by default (docker only manages FORWARD),
  # which breaks in-cluster DNS + image pulls. Accept INPUT from docker bridges.
  networking.firewall.extraCommands = ''
    iptables  -I nixos-fw 1 -i br-+ -j nixos-fw-accept
    ip6tables -I nixos-fw 1 -i br-+ -j nixos-fw-accept
  '';
  networking.firewall.extraStopCommands = ''
    iptables  -D nixos-fw -i br-+ -j nixos-fw-accept 2>/dev/null || true
    ip6tables -D nixos-fw -i br-+ -j nixos-fw-accept 2>/dev/null || true
  '';
  programs.nix-ld.enable = true;       # let prebuilt dynamically-linked tools run on NixOS
  boot.kernel.sysctl = {
    "fs.inotify.max_user_watches" = 1048576; # kind needs many watches/instances
    "fs.inotify.max_user_instances" = 8192;
  };

  # Gardener/fundament edit /etc/hosts at runtime (e.g. registry.local.gardener.cloud).
  # NixOS makes /etc/hosts a read-only /nix/store symlink by default; copy it instead
  # so runtime `sudo tee -a /etc/hosts` succeeds. Edits persist across reboot, which is
  # fine here — the entries are deterministic and idempotently re-added by gardener-up.
  environment.etc."hosts".mode = "0644";

  # registry.local.gardener.cloud is the host-side push target for Gardener's local
  # registry (HTTP, :5001). Bake it in so it survives nixos-rebuild (which regenerates
  # /etc/hosts) and clean reboots; gardener-up otherwise adds it at runtime, but a
  # rebuild would wipe that. Other *.local.gardener.cloud names resolve inside the
  # clusters (k3d/kind CoreDNS), not on the host.
  networking.extraHosts = "127.0.0.1 registry.local.gardener.cloud";

  # Gardener's local DNS: every *.local.gardener.cloud name except the registry (the
  # virtual-garden API, shoot APIs, ...) is served by the in-cluster bind9 at
  # 172.18.255.53. The host must forward that zone to bind9. Gardener's kind-up.sh
  # writes exactly this systemd-resolved drop-in — but only when resolved is already
  # active, which it isn't on a plain resolvconf box, so it silently skips (the DNS
  # fragility we hit). Enable resolved + ship the drop-in to make it deterministic.
  # resolved reads /etc/hosts first, so registry.local.gardener.cloud still wins above.
  services.resolved.enable = true;
  environment.etc."systemd/resolved.conf.d/gardener-local.conf".text = ''
    [Resolve]
    DNS=172.18.255.53 fd00:ff::53
    Domains=~local.gardener.cloud
  '';
  # Scripted networking (dhcpcd, via networking.useDHCP) has no systemd-resolved
  # integration, so the DHCP DNS never reaches resolved and general resolution breaks
  # at boot (the gardener routing domain above is global; everything else needs the
  # link's DHCP DNS). A dhcpcd hook is unreliable (runs unprivileged), so register it
  # from a root oneshot once the network + resolved are up: find the default-route
  # interface, read its lease DNS, and hand it to resolved.
  systemd.services.resolved-uplink-dns = {
    description = "Register the DHCP-provided DNS with systemd-resolved";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "systemd-resolved.service" ];
    wants = [ "network-online.target" ];
    requires = [ "systemd-resolved.service" ];
    path = [ pkgs.iproute2 pkgs.dhcpcd pkgs.systemd pkgs.gawk pkgs.gnused pkgs.coreutils ];
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
    script = ''
      for i in $(seq 1 30); do
        iface=$(ip -o route show default | awk '{for(j=1;j<NF;j++) if($j=="dev"){print $(j+1); exit}}')
        if [ -n "$iface" ]; then
          dns=$(dhcpcd -U "$iface" 2>/dev/null | sed -n "s/^domain_name_servers=//p" | tr -d "'")
          if [ -n "$dns" ]; then
            resolvectl dns "$iface" $dns
            exit 0
          fi
        fi
        sleep 2
      done
      echo "resolved-uplink-dns: no DHCP DNS found to register" >&2
    '';
  };

  environment.systemPackages = with pkgs; [
    git just jq
    docker-compose
    kubectl kubernetes-helm
    kind k3d
    nssTools # certutil — mkcert (setup-certs) needs it to manage the NSS CA store
    yq-go openssl # Gardener's local setup needs these (it's Makefile-driven)
    mise # toolchain manager; reads the fundament repo's mise.toml (go, kubectl, skaffold, ...)
    gnumake # Gardener's local setup is Makefile-driven
    # General C/C++ build toolchain for a dev box (cgo, native deps). NOTE: mise uses a
    # PREBUILT node (MISE_NODE_COMPILE=0 in box/*.sh) run via nix-ld, so node is NOT built
    # from source — these aren't needed for that. Kept for a complete dev box; drop for a leaner image.
    gcc python3
  ];

  # --- Nix ----------------------------------------------------------------
  nix.settings.experimental-features = [ "nix-command" "flakes" ];
  nix.settings.trusted-users = [ "root" user ];

  time.timeZone = "Europe/Amsterdam";

  # Set to the NixOS release you install FROM. Do not bump casually.
  system.stateVersion = "25.11";
}
