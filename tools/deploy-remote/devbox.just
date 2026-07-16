# Daily-cattle personal devbox on Hetzner Cloud: box destroyed nightly, your
# state (repos, WIP, ~/.claude, caches) on a persistent per-dev volume.
# Thin wrappers over ./hetzner.sh with the devbox role. Run from the repo root
# as `just devbox <recipe>`.

export HZ_ROLE := "devbox"

_default:
    @just --list

# morning: create box + attach your volume (+ optionally the stack: --stack mock|gardener)
up *flags:
    ./hetzner.sh up {{ flags }}

# evening: destroy the box (warns about un-pushed work; the volume SURVIVES)
down:
    ./hetzner.sh down

# bring up / switch the stack on the running box: mock (no Gardener) or gardener
stack mode:
    ./hetzner.sh stack {{ mode }}

# ssh into your devbox (port 2022)
ssh:
    ./hetzner.sh ssh

# foreground tunnel for https://*.fundament.localhost:8443 (or just `ssh devbox-<you>`)
tunnel:
    ./hetzner.sh tunnel

# regenerate the managed ssh config block (up does this automatically)
ssh-config:
    ./hetzner.sh ssh-config

# servers AND volumes — both bill
status:
    ./hetzner.sh status

# re-trust the devbox CA locally (up does this; use from a second machine)
certs:
    ./hetzner.sh certs

# DELETE your persistent volume (asks for confirmation; repos/logins on it are gone)
destroy-volume:
    ./hetzner.sh destroy-volume
