#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

(
  cd manifest-frontend
  npm run build
)

go run ./tools/generate-embedded-bridges
go build -tags tspack_embedded_bridges -o dist/tspack ./cmd/tspack

if [ -d manifest-frontend/dist ]; then
  mv manifest-frontend/dist manifest-frontend/dist.release-smoke-bak
  restore_dist() {
    if [ -d manifest-frontend/dist.release-smoke-bak ]; then
      rm -rf manifest-frontend/dist
      mv manifest-frontend/dist.release-smoke-bak manifest-frontend/dist
    fi
  }
  trap restore_dist EXIT
fi

./dist/tspack check --root examples/runtime-switch-notes
./dist/tspack test --root examples/runtime-switch-notes 2>dist/test-smoke.err || {
  if grep -q "BRIDGE_MISSING" dist/test-smoke.err; then
    cat dist/test-smoke.err >&2
    exit 1
  fi
}
./dist/tspack inspect examples/runtime-switch-notes --json >/dev/null 2>dist/inspect-smoke.err || {
  if grep -q "BRIDGE_MISSING" dist/inspect-smoke.err; then
    cat dist/inspect-smoke.err >&2
    exit 1
  fi
}
