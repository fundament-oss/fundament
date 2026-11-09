#!/usr/bin/env bash
# Packs what a failed run needs into $STATE/bundles/<time>-<sha>.tar.gz: the e2e
# report with traces, logs of fundament, Gardener and the shoots, the shoot
# objects, the journal and the memory samples. The host fetches it.
set -uo pipefail
. "$(dirname "$0")/lib.sh"

name="$(date -u +%Y%m%dT%H%M%SZ)-$(git -C "$REPO" rev-parse --short HEAD)"
dir=$(mktemp -d)/$name
mkdir -p "$dir"/{fundament,seed,garden}
cd "$dir" || exit 1

[ -d "$REPO/e2e/reports" ] && cp -r "$REPO/e2e/reports" e2e-reports
cp "$STATE"/logs/* . 2>/dev/null
{
  awk -F, 'NR>1 && $2>m {m=$2; t=$1} END {print "peak used: " m " MiB at " t}' mem.csv
  awk -F, 'NR>1 && $3>m {m=$3} END {print "peak load1: " m}' mem.csv
  echo "peak per container (MiB):"
  awk -F, 'NR>1 && $3>p[$2] {p[$2]=$3} END {for (c in p) printf "  %-45s %d\n", c, p[c]}' containers.csv | sort -k2 -nr
} > resources.txt 2>/dev/null
git -C "$REPO" rev-parse HEAD > sha.txt

k3d=(kubectl --context k3d-fundament --request-timeout=20s)
"${k3d[@]}" -n fundament get all,events -o wide > fundament/resources.txt 2>&1
for pod in $("${k3d[@]}" -n fundament get pods -o name 2>/dev/null); do
  "${k3d[@]}" -n fundament logs "$pod" --all-containers --prefix > "fundament/${pod#pod/}.log" 2>&1
  "${k3d[@]}" -n fundament logs "$pod" --all-containers --prefix --previous > "fundament/${pod#pod/}.previous.log" 2>/dev/null \
    || rm -f "fundament/${pod#pod/}.previous.log"
done

garden=(kubectl --context "$GARDEN_CONTEXT" --request-timeout=20s)
"${garden[@]}" get shoots -A -o yaml > garden/shoots.yaml 2>&1
"${garden[@]}" get seeds -o yaml > garden/seeds.yaml 2>&1
"${garden[@]}" get events -A > garden/events.txt 2>&1

seed=(kubectl --context "$SEED_CONTEXT" --request-timeout=20s)
"${seed[@]}" get garden -o yaml > seed/garden.yaml 2>&1
for ns in $("${seed[@]}" get ns -o name 2>/dev/null | sed 's|namespace/||' | grep -E '^(garden|shoot--)'); do
  mkdir -p "seed/$ns"
  "${seed[@]}" -n "$ns" get all,events -o wide > "seed/$ns/resources.txt" 2>&1
  for pod in $("${seed[@]}" -n "$ns" get pods -o name 2>/dev/null); do
    "${seed[@]}" -n "$ns" logs "$pod" --all-containers --prefix --tail=5000 > "seed/$ns/${pod#pod/}.log" 2>&1
  done
done

journalctl -b --no-pager > journal.txt 2>&1
docker ps -a > docker-ps.txt 2>&1

tar -czf "$STATE/bundles/$name.tar.gz" -C "$(dirname "$dir")" "$name" \
  || { rm -f "$STATE/bundles/$name.tar.gz"; die "packing the bundle failed; the files stay in $dir"; }
rm -rf "$(dirname "$dir")"
log "bundle $STATE/bundles/$name.tar.gz"
