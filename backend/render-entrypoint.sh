#!/bin/sh
set -eu

if [ "${RUN_MIGRATIONS:-false}" = "true" ]; then
  migrate -path /migrations -database "$DATABASE_URL" up
fi

exec "$@"
