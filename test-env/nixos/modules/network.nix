# DHCP on the uplink through systemd-networkd, which hands the lease's DNS to
# systemd-resolved, plus the forwarding of Gardener's local DNS zone.
{ pkgs, ... }:
{
  networking.useNetworkd = true;
  networking.useDHCP = false;
  systemd.network.networks."10-uplink" = {
    # Docker, kind and k3d bridges (docker0, br-*, veth*) stay unmanaged.
    matchConfig.Name = "en* eth*";
    networkConfig.DHCP = "ipv4";
    # IPv4 only, the protocol docker-uplink-filter covers.
    networkConfig.IPv6AcceptRA = false;
    networkConfig.LinkLocalAddressing = "no";
  };

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

  # Docker publishes container ports through DNAT and FORWARD, past the NixOS
  # firewall, and the k3d registry, Postgres and Gardener's registry publish on
  # every address. Nothing outside the VM may open a connection to them: on a
  # cloud host the uplink is the internet.
  systemd.services.docker-uplink-filter = {
    description = "Drop new connections from the uplink to containers";
    after = [ "docker.service" ];
    bindsTo = [ "docker.service" ];
    wantedBy = [ "docker.service" ];
    path = [ pkgs.iptables ];
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
    script = ''
      for uplink in en+ eth+; do
        rule=(DOCKER-USER -i "$uplink" -m conntrack --ctstate NEW -j DROP)
        iptables -C "''${rule[@]}" 2>/dev/null || iptables -I "''${rule[@]}"
      done
    '';
  };

  # Every *.local.gardener.cloud name except the registry (the virtual-garden API,
  # shoot APIs, ...) is served by the in-cluster bind9 at 172.18.255.53. Gardener's
  # kind-up.sh writes this drop-in only when resolved is already active and skips it
  # silently otherwise, so it is shipped here.
  services.resolved.enable = true;
  environment.etc."systemd/resolved.conf.d/gardener-local.conf".text = ''
    [Resolve]
    DNS=172.18.255.53 fd00:ff::53
    Domains=~local.gardener.cloud
  '';
}
