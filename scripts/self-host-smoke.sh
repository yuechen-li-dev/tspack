#!/usr/bin/env sh
set -eu

usage() {
  cat <<'USAGE'
usage: ./scripts/self-host-smoke.sh [--release|--help]

Runs the routine self-host dogfood smoke from the repository root.

Modes:
  default     Build the manifest frontend if needed, run the read-only
              self-host command matrix, and fail if tracked files change.
  --release   Also run the release build script. This may create ignored
              release artifacts and is intended for release/manual gates.
  --help      Show this help.

Ignored generated artifacts may be created. Tracked repository state must not
change during the smoke.
USAGE
}

fail() {
  echo "self-host smoke: $*" >&2
  exit 1
}

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required tool '$1' was not found on PATH"
  fi
}

run_release=0
case "${1:-}" in
  "")
    ;;
  --release)
    run_release=1
    ;;
  --help|-h)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [ "$#" -gt 1 ]; then
  usage >&2
  exit 2
fi

[ -f manifest.tsx ] || fail "must be run from the repository root containing manifest.tsx"
[ -f go.mod ] || fail "must be run from the repository root containing go.mod"
[ -d manifest-frontend ] || fail "manifest-frontend directory is missing"

require_tool git
require_tool npm
require_tool go

ensure_biome_backend() {
  if command -v biome >/dev/null 2>&1; then
    return
  fi

  mkdir -p .tspack/self-host-bin
  cat > .tspack/self-host-bin/biome <<'BIOME_WRAPPER'
#!/usr/bin/env sh
exec npm exec --yes --package @biomejs/biome@2.5.1 -- biome "$@"
BIOME_WRAPPER
  chmod +x .tspack/self-host-bin/biome
  PATH="$(pwd)/.tspack/self-host-bin:$PATH"
  export PATH
  echo "self-host smoke: biome backend not found on PATH; using ignored npm-exec wrapper"
}

before_status=$(git status --short --untracked-files=no)

echo "self-host smoke: building manifest frontend bootstrap bridge"
npm --prefix manifest-frontend run build
ensure_biome_backend

echo "self-host smoke: running read-only command matrix"
go run ./cmd/tspack run --list --root .
go run ./cmd/tspack check --root .
go run ./cmd/tspack check --format --root .
go run ./cmd/tspack doctor security --root . --json
go run ./cmd/tspack update --policy --dry-run --root . --json

if [ "$run_release" -eq 1 ]; then
  echo "self-host smoke: running optional release build"
  ./scripts/build-release.sh
fi

after_status=$(git status --short --untracked-files=no)
if [ "$before_status" != "$after_status" ]; then
  echo "self-host smoke: tracked repository state changed" >&2
  echo "self-host smoke: ignored generated artifacts are allowed, but tracked files must not change" >&2
  git status --short --untracked-files=no >&2
  exit 1
fi

echo "self-host smoke: passed with no tracked repository mutation"
