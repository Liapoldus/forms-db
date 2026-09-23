#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CORE_ROOT=${LIAPOLDUS_CORE_ROOT:-"$ROOT/../../core"}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/liapoldus-forms-smoke.XXXXXX")
GATEWAY_PID=
cleanup() {
  if [ -n "$GATEWAY_PID" ]; then
    kill "$GATEWAY_PID" 2>/dev/null || true
    wait "$GATEWAY_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT INT TERM
if ! go build -o "$TMP/forms-db" ./cmd/forms-db; then exit 1; fi
if ! (cd "$CORE_ROOT" && go build -o "$TMP/gateway" ./cmd/gateway); then exit 1; fi
printf '%s\n' \
  'registry:' "  path: $TMP/registry" 'plugins:' '  forms-db:' \
  "    binary: $TMP/forms-db" '    capabilities: [forms.submit]' '    settings: {}' \
  'listeners:' '  web:' '    type: http' '    address: 127.0.0.1:18101' '    routes:' \
  '      - when: { path: { exact: /smoke } }' \
  '        then: { plugin: { instance: forms-db, capability: forms.submit } }' > "$TMP/gateway.yaml"
"$TMP/gateway" --config "$TMP/gateway.yaml" serve --no-management >"$TMP/gateway.out" 2>&1 &
GATEWAY_PID=$!
READY=0
for _ in $(seq 1 120); do
  if curl -sS -o /dev/null http://127.0.0.1:18101/missing 2>/dev/null; then READY=1; break; fi
  sleep 0.05
done
test "$READY" = 1
response=$(curl -sS -X POST http://127.0.0.1:18101/smoke -H 'Content-Type: application/json' -d '{"site":"portal","schemaName":"contact","data":{"name":"fixture"}}')
printf '%s' "$response" | grep -Eq '"id":"frm_[A-Za-z0-9_-]+"'
