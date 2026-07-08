#!/bin/sh
set -eu

api_pid=""
web_pid=""

cleanup() {
  if [ -n "$web_pid" ]; then
    kill "$web_pid" 2>/dev/null || true
  fi
  if [ -n "$api_pid" ]; then
    kill "$api_pid" 2>/dev/null || true
  fi
}

trap 'cleanup; exit 143' INT TERM

node server.js &
web_pid=$!

missing_backend_config=false
for name in DATABASE_URL S3_ENDPOINT S3_PUBLIC_ENDPOINT S3_ACCESS_KEY S3_SECRET_KEY S3_BUCKET S3_REGION; do
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "Missing $name; local API in the web container will not start."
    missing_backend_config=true
  fi
done

if [ -z "${CREDENTIALS_ENCRYPTION_KEY:-}" ] && [ -z "${API_SHARED_SECRET:-}" ] && [ -z "${GEMINI_API_KEY:-}" ]; then
  echo "Missing CREDENTIALS_ENCRYPTION_KEY, API_SHARED_SECRET, or GEMINI_API_KEY; local API in the web container will not start."
  missing_backend_config=true
fi

if [ "$missing_backend_config" = "false" ]; then
  if [ "${RUN_MIGRATIONS:-false}" = "true" ]; then
    migrate -path /migrations -database "$DATABASE_URL" up
  fi

  export INTERNAL_API_URL=http://127.0.0.1:8787
  api &
  api_pid=$!
else
  unset INTERNAL_API_URL
  case "${API_URL:-}" in
    http://127.0.0.1*|http://localhost*|http://\[::1\]*)
      unset API_URL
      ;;
  esac
fi

while true; do
  if [ -n "$api_pid" ] && ! kill -0 "$api_pid" 2>/dev/null; then
    wait "$api_pid" || true
    kill "$web_pid" 2>/dev/null || true
    wait "$web_pid" 2>/dev/null || true
    exit 1
  fi
  if ! kill -0 "$web_pid" 2>/dev/null; then
    set +e
    wait "$web_pid"
    status=$?
    set -e
    if [ -n "$api_pid" ]; then
      kill "$api_pid" 2>/dev/null || true
      wait "$api_pid" 2>/dev/null || true
    fi
    exit "$status"
  fi
  sleep 2
done
