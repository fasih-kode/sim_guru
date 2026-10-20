#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pg_bin="$(pg_config --bindir)"
data="$root/.local/postgres/data"
socket="$root/.local/postgres/socket"
port=55436
case "${1:-status}" in
  start)
    test -f "$data/PG_VERSION" || { echo 'Cluster belum diinisialisasi; lihat docs/DEVELOPMENT.md.' >&2; exit 1; }
    mkdir -p "$socket"
    if "$pg_bin/pg_ctl" -D "$data" status >/dev/null 2>&1; then
      echo 'Cluster proyek sudah aktif.'
    else
      "$pg_bin/pg_ctl" -D "$data" -l "$root/.local/postgres/server.log" -o "-h 127.0.0.1 -p $port -k $socket" -w start
    fi
    ;;
  status) "$pg_bin/pg_ctl" -D "$data" status ;;
  stop) "$pg_bin/pg_ctl" -D "$data" -m smart -w stop ;;
  *) echo 'Gunakan: scripts/dev-db.sh start|status|stop' >&2; exit 2 ;;
esac
