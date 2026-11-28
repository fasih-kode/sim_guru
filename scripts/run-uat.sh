#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
config="${SIM_GURU_UAT_CONFIG:-${HOME}/.config/sim-guru/uat.env}"

if [[ ! -f "$config" || -L "$config" ]]; then
  echo "Konfigurasi UAT privat tidak tersedia atau bukan file biasa." >&2
  exit 1
fi
config="$(realpath "$config")"
case "$config" in
  "$root"/*)
    echo "Konfigurasi UAT wajib berada di luar repository." >&2
    exit 1
    ;;
esac
if [[ "$(stat -c '%u:%a' "$config")" != "$(id -u):600" ]]; then
  echo "Konfigurasi UAT harus dimiliki pengguna aktif dan bermode 0600." >&2
  exit 1
fi
if [[ "$(stat -c '%u:%a' "$(dirname "$config")")" != "$(id -u):700" ]]; then
  echo "Direktori konfigurasi UAT harus dimiliki pengguna aktif dan bermode 0700." >&2
  exit 1
fi

# File konfigurasi milik pengguna lokal, mode 0600, dan berada di luar repo.
set -a
# shellcheck disable=SC1090
source "$config"
set +a

if ! python3 - "$root/.env" <<'PY'
import os
import sys
from urllib.parse import urlsplit

def read_env(path):
    values = {}
    try:
        lines = open(path, encoding="utf-8")
    except FileNotFoundError:
        return values
    with lines:
        for raw in lines:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            value = value.strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
                value = value[1:-1]
            values[key.strip()] = value
    return values

dsn = urlsplit(os.environ.get("DATABASE_URL", ""))
if (dsn.scheme not in ("postgres", "postgresql") or dsn.hostname != "127.0.0.1"
        or dsn.port != 55436 or dsn.path != "/sim_guru_ci_test"):
    print("Target UAT wajib PostgreSQL lokal 55436 / sim_guru_ci_test.", file=sys.stderr)
    raise SystemExit(1)
if os.environ.get("APP_HOST") != "localhost" or os.environ.get("APP_PORT") != "18744":
    print("UAT wajib bind pada localhost:18744.", file=sys.stderr)
    raise SystemExit(1)
if os.environ.get("APP_ENV") != "development":
    print("APP_ENV UAT wajib development untuk konfigurasi aplikasi lokal.", file=sys.stderr)
    raise SystemExit(1)
if len(os.environ.get("CSRF_AUTH_KEY", "")) != 64:
    print("Kunci CSRF UAT tidak valid.", file=sys.stderr)
    raise SystemExit(1)
dev = read_env(sys.argv[1])
if not dev.get("CSRF_AUTH_KEY") or dev["CSRF_AUTH_KEY"] == os.environ["CSRF_AUTH_KEY"]:
    print("Kunci CSRF UAT harus berbeda dari kunci development.", file=sys.stderr)
    raise SystemExit(1)
for key in ("TEACHER_PHOTO_DIR", "STAFF_PHOTO_DIR", "APP_REPOSITORY_ROOT", "APP_TRUSTED_PROXY_CIDRS"):
    if os.environ.get(key, ""):
        print("Storage foto dan proxy tepercaya harus nonaktif pada preview UAT lokal.", file=sys.stderr)
        raise SystemExit(1)
PY
then
  exit 1
fi

if ss -H -ltn 'sport = :18744' | grep -q .; then
  echo "Port UAT 18744 sedang digunakan; proses yang ada tidak dihentikan otomatis." >&2
  exit 1
fi

cd "$root"
if ! python3 scripts/check_migration_status.py; then
  echo "Database UAT harus sudah mencapai target migration source; tidak ada migration otomatis." >&2
  exit 1
fi

echo "Preview UAT berjalan di http://localhost:18744 dengan storage foto dan worker retensi nonaktif. Hentikan hanya proses ini dengan Ctrl+C."
exec go run ./cmd/server
