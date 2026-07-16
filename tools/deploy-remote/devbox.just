# Daily-cattle personal devbox on Hetzner Cloud: box destroyed nightly, your
# state (repos, WIP, ~/.claude, caches) on a persistent per-dev volume.
# Thin wrappers over ./hetzner.sh with the devbox role. Run from the repo root
# as `just devbox <recipe>`.

_default:
    @just --list

# morning: create box + attach your volume (+ optionally the stack: --stack mock|gardener)
up *flags:
    HZ_ROLE=devbox ./hetzner.sh up {{ flags }}

# evening: destroy the box (warns about un-pushed work; the volume SURVIVES)
down:
    HZ_ROLE=devbox ./hetzner.sh down

# bring up / switch the stack on the running box: mock (no Gardener) or gardener
stack mode:
    HZ_ROLE=devbox ./hetzner.sh stack {{ mode }}

# ssh into your devbox (port 2022)
ssh:
    HZ_ROLE=devbox ./hetzner.sh ssh

# foreground tunnel for https://*.fundament.localhost:8443 (or just `ssh devbox-<you>`)
tunnel:
    HZ_ROLE=devbox ./hetzner.sh tunnel

# regenerate the managed ssh config block (up does this automatically)
ssh-config:
    HZ_ROLE=devbox ./hetzner.sh ssh-config

# servers AND volumes — both bill
status:
    HZ_ROLE=devbox ./hetzner.sh status

# re-trust the devbox CA locally (up does this; use from a second machine)
certs:
    HZ_ROLE=devbox ./hetzner.sh certs

# DELETE your persistent volume (asks for confirmation; repos/logins on it are gone)
destroy-volume:
    HZ_ROLE=devbox ./hetzner.sh destroy-volume
