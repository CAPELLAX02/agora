#!/bin/sh
# Uçtan uca testler için boş bir veritabanı (agora_e2e) kurar: şema ve demo verileri.
# Geliştirme veritabanına dokunmaz. psql varsa (CI) doğrudan, yoksa compose'daki
# PostgreSQL konteyneri üzerinden çalışır.
set -eu

cd "$(dirname "$0")/../.."

DB=agora_e2e
URL="postgres://agora:agora_dev_password@localhost:5432"

if command -v psql >/dev/null 2>&1; then
  psql_admin() { psql -v ON_ERROR_STOP=1 -q "$URL/postgres" "$@"; }
  psql_e2e() { psql -v ON_ERROR_STOP=1 -q "$URL/$DB" "$@"; }
else
  psql_admin() { docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -q -U agora -d postgres "$@"; }
  psql_e2e() { docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -q -U agora -d "$DB" "$@"; }
fi

psql_admin -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)" -c "CREATE DATABASE $DB"

export AGORA_DATABASE_URL="$URL/$DB?sslmode=disable"
export AGORA_REDIS_URL="redis://localhost:6379/1"
go -C backend run ./cmd/migrate up >/dev/null

for f in infra/seed/dev/*.sql; do
  psql_e2e <"$f"
done
go -C backend run ./cmd/seed >/dev/null
echo "e2e veritabanı hazır: $DB"
