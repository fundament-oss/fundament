# Samples resource use for the run bundle, so the VM size can be set from what a
# run needs: memory and load every 10 s, memory per container every minute.
{ pkgs, ... }:
{
  systemd.services.test-env-memlog = {
    description = "Sample resource use for the run bundle";
    wantedBy = [ "multi-user.target" ];
    after = [ "docker.service" ];
    path = [ pkgs.coreutils pkgs.gawk pkgs.docker ];
    serviceConfig.Restart = "always";
    script = ''
      mkdir -p /var/lib/test-env/logs
      out=/var/lib/test-env/logs/mem.csv
      containers=/var/lib/test-env/logs/containers.csv
      [ -s "$out" ] || echo "time,used_mib,load1" > "$out"
      [ -s "$containers" ] || echo "time,container,mem_mib,cpu_percent" > "$containers"
      n=0
      while sleep 10; do
        used=$(awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print int((t-a)/1024)}' /proc/meminfo)
        echo "$(date -Is),$used,$(cut -d' ' -f1 /proc/loadavg)" >> "$out"
        n=$(( (n + 1) % 6 ))
        [ "$n" -eq 0 ] || continue
        now=$(date -Is)
        docker stats --no-stream --format '{{.Name}},{{.MemUsage}},{{.CPUPerc}}' 2>/dev/null \
          | awk -F, -v t="$now" '{
              split($2, m, " ");
              v = m[1]; mib = v + 0;
              if (v ~ /GiB/) mib *= 1024; else if (v ~ /KiB/) mib /= 1024; else if (v ~ /B$/ && v !~ /MiB/) mib /= 1048576;
              sub(/%/, "", $3);
              printf "%s,%s,%d,%s\n", t, $1, mib, $3
            }' >> "$containers"
      done
    '';
  };
}
