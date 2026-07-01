#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

hidden_dist_backup="manifest-frontend/dist.release-smoke-bak"
dist_was_hidden=0

cleanup() {
  if [ "$dist_was_hidden" -eq 1 ] && [ -d "$hidden_dist_backup" ]; then
    if [ -e manifest-frontend/dist ]; then
      preserved_path="manifest-frontend/dist.release-smoke-unexpected-$(date +%s)-$$"
      echo "Preserving unexpected manifest-frontend/dist created during smoke at $preserved_path" >&2
      mv manifest-frontend/dist "$preserved_path"
    fi
    mv "$hidden_dist_backup" manifest-frontend/dist
  fi
}
trap cleanup EXIT INT TERM

fail_with_output() {
  local label="$1"
  local output_file="$2"

  echo "$label failed:" >&2
  cat "$output_file" >&2
  exit 1
}

assert_no_bridge_missing() {
  local label="$1"
  local output_file="$2"

  if grep -Eqi 'TSPACK_BRIDGE_UNAVAILABLE|TSPACK_.*BRIDGE.*NOT_FOUND|TSPACK_.*BRIDGE_MISSING|manifest frontend bridge not found|inspect bridge not found|native xTest bridge not found' "$output_file"; then
    fail_with_output "$label reported a missing bridge" "$output_file"
  fi
}

assert_expected_inspect_diagnostic() {
  local output_file="$1"

  if grep -Eq 'TSPACK_INSPECT_(BROWSER_LAUNCH_FAILED|VSCODE_NOT_FOUND|PAGE_LOAD_FAILED|PLAYWRIGHT_CORE_NOT_FOUND|PLAYWRIGHT_CORE_LOAD_FAILED|INVALID_TARGET|FAILED)' "$output_file"; then
    return 0
  fi

  fail_with_output "inspect smoke did not reach a stable post-bridge diagnostic" "$output_file"
}

(
  cd manifest-frontend
  npm run build
)

go run ./tools/generate-embedded-bridges
mkdir -p dist
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
version="${TSPACK_VERSION:-v0.1.7}"
commit="${GITHUB_SHA:-$(git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)}"

go build \
  -tags tspack_embedded_bridges \
  -ldflags "-X github.com/yuechen-li-dev/tspack/internal/version.Version=$version -X github.com/yuechen-li-dev/tspack/internal/version.Commit=$commit -X github.com/yuechen-li-dev/tspack/internal/version.Date=$build_date" \
  -o dist/tspack \
  ./cmd/tspack

if [ -e "$hidden_dist_backup" ]; then
  echo "$hidden_dist_backup already exists; remove or restore it before running release smoke." >&2
  exit 1
fi

if [ -d manifest-frontend/dist ]; then
  mv manifest-frontend/dist "$hidden_dist_backup"
  dist_was_hidden=1
fi

./dist/tspack check --root examples/runtime-switch-notes

if ! ./dist/tspack test --root examples/runtime-switch-notes >dist/test-smoke.out 2>dist/test-smoke.err; then
  cat dist/test-smoke.out dist/test-smoke.err >dist/test-smoke.log
  assert_no_bridge_missing "test smoke" dist/test-smoke.log
fi

if ! ./dist/tspack inspect http://127.0.0.1:9 --json >dist/inspect-smoke.out 2>dist/inspect-smoke.err; then
  cat dist/inspect-smoke.out dist/inspect-smoke.err >dist/inspect-smoke.log
  assert_no_bridge_missing "inspect smoke" dist/inspect-smoke.log
  assert_expected_inspect_diagnostic dist/inspect-smoke.log
fi
