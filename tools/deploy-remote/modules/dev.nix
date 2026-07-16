# Devbox role: the developer layer on top of baseline.nix. System-level only —
# everything per-user (repo, mise toolchain, Claude Code, certs) is installed by
# box/bootstrap-dev.sh into $HOME, which lives on the persistent volume.
{ pkgs, ... }:
{
  environment.systemPackages = with pkgs; [
    # Terminal editors (VS Code Remote-SSH needs nothing beyond baseline's nix-ld)
    helix
    neovim
    # Claude Code's Linux sandbox mode execs these to confine file/network access
    # (bubblewrap = user-namespace sandbox from Flatpak, socat = socket relay).
    bubblewrap
    socat
    # Daily-driver CLI basics
    gh
    direnv
    ripgrep
    fd
    fzf
    htop
    tmux
  ];

  # tmux is available but never assumed — plain multi-session SSH is first-class.
  # This config only matters for devs who opt in (e.g. to keep a Claude Code
  # session alive across a laptop disconnect): passthrough + extended keys are
  # what Claude Code's TUI needs (shift+enter etc.).
  programs.tmux = {
    enable = true;
    extraConfig = ''
      set -g allow-passthrough on
      set -s extended-keys on
      set -as terminal-features 'xterm*:extkeys'
    '';
  };
}
