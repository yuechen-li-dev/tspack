#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/package-release.sh --goos <goos> --goarch <goarch> [--version <version>] [--out-dir <dir>]

Builds a self-contained release binary with embedded bridge assets and packages it
as tspack-<goos>-<goarch>.tar.gz or tspack-windows-<goarch>.zip.
USAGE
}

goos=""
goarch=""
version="dev"
out_dir="dist/release"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --goos)
      goos="${2:-}"
      shift 2
      ;;
    --goarch)
      goarch="${2:-}"
      shift 2
      ;;
    --version)
      version="${2:-}"
      shift 2
      ;;
    --out-dir)
      out_dir="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [ -z "$goos" ] || [ -z "$goarch" ]; then
  echo "--goos and --goarch are required." >&2
  usage
  exit 2
fi

case "$goos/$goarch" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64)
    ;;
  *)
    echo "Unsupported release target: $goos/$goarch" >&2
    exit 2
    ;;
esac

if [ -f manifest-frontend/package-lock.json ]; then
  (cd manifest-frontend && npm ci)
elif [ ! -d manifest-frontend/node_modules ]; then
  (cd manifest-frontend && npm install --no-package-lock)
fi

(cd manifest-frontend && npm run build)
go run ./tools/generate-embedded-bridges

package_name="tspack-${goos}-${goarch}"
work_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT INT TERM

package_dir="$work_dir/$package_name"
mkdir -p "$package_dir" "$out_dir"

binary_name="tspack"
archive_name="${package_name}.tar.gz"
if [ "$goos" = "windows" ]; then
  binary_name="tspack.exe"
  archive_name="${package_name}.zip"
fi

build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
commit="${GITHUB_SHA:-$(git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)}"

GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build \
  -tags tspack_embedded_bridges \
  -trimpath \
  -ldflags "-X github.com/tspack/tspack/internal/version.Version=$version -X github.com/tspack/tspack/internal/version.Commit=$commit -X github.com/tspack/tspack/internal/version.Date=$build_date" \
  -o "$package_dir/$binary_name" \
  ./cmd/tspack

chmod 0755 "$package_dir/$binary_name"

if [ -f LICENSE ]; then
  cp LICENSE "$package_dir/LICENSE"
fi

if [ -f README.md ]; then
  cp README.md "$package_dir/README.md"
else
  cat > "$package_dir/README.md" <<EOF_README
# TSPack $version

This archive contains the TSPack command-line binary for $goos/$goarch.
EOF_README
fi

archive_path="$out_dir/$archive_name"
rm -f "$archive_path"

if [ "$goos" = "windows" ]; then
  (cd "$work_dir" && zip -qr "$repo_root/$archive_path" "$package_name")
else
  (cd "$work_dir" && tar -czf "$repo_root/$archive_path" "$package_name")
fi

echo "$archive_path"
