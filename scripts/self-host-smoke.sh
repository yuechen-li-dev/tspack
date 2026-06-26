#!/usr/bin/env sh
set -eu

run_release=0
if [ "${1:-}" = "--release" ]; then
  run_release=1
elif [ "${1:-}" != "" ]; then
  echo "usage: $0 [--release]" >&2
  exit 2
fi

before_status=$(git status --short --untracked-files=no)

npm --prefix manifest-frontend run build
go run ./cmd/tspack sync --root .

go run ./cmd/tspack run --list --root .
go run ./cmd/tspack check --root .
go run ./cmd/tspack check --format --root .
go run ./cmd/tspack doctor security --root . --json
go run ./cmd/tspack update --policy --dry-run --root . --json

if [ "$run_release" -eq 1 ]; then
  go run ./cmd/tspack run release-build --root . --once --ready-timeout 300
fi

after_status=$(git status --short --untracked-files=no)
if [ "$before_status" != "$after_status" ]; then
  echo "self-host smoke changed tracked repository state" >&2
  git status --short --untracked-files=no >&2
  exit 1
fi
