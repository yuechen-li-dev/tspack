#!/usr/bin/env sh
set -eu

log() {
  printf '%s\n' "$*"
}

err() {
  printf 'tspack install: %s\n' "$*" >&2
}

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/install.sh [--version <version>] [--install-dir <dir>]

Downloads a TSPack release artifact from GitHub Releases, verifies its SHA256
checksum, and installs the tspack binary.

Environment overrides:
  TSPACK_VERSION       Release tag to install, for example v0.1.5.
  TSPACK_INSTALL_DIR   Install directory. Defaults to $HOME/.local/bin.
  TSPACK_REPO          GitHub repo in owner/name form. Defaults to yuechen-li-dev/tspack.
  TSPACK_GITHUB_BASE   GitHub web base URL. Defaults to https://github.com.
  TSPACK_API_BASE      GitHub API base URL. Defaults to https://api.github.com.
USAGE
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

fail_missing_value() {
  err "$1 requires a value."
  usage
  exit 2
}

install_dir="${TSPACK_INSTALL_DIR:-${HOME}/.local/bin}"
version="${TSPACK_VERSION:-}"
repo="${TSPACK_REPO:-yuechen-li-dev/tspack}"
github_base="${TSPACK_GITHUB_BASE:-https://github.com}"
api_base="${TSPACK_API_BASE:-https://api.github.com}"

dry_run="${TSPACK_INSTALL_DRY_RUN:-0}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail_missing_value "--version"
      version="$2"
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail_missing_value "--install-dir"
      install_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      err "Unknown argument: $1"
      usage
      exit 2
      ;;
  esac
done

os_name=""
arch_name=""
artifact=""
work_dir=""

cleanup() {
  if [ -n "$work_dir" ] && [ -d "$work_dir" ]; then
    rm -rf "$work_dir"
  fi
}
trap cleanup EXIT INT TERM

detect_platform() {
  raw_os="${TSPACK_TEST_OS:-$(uname -s)}"
  raw_arch="${TSPACK_TEST_ARCH:-$(uname -m)}"

  case "$raw_os" in
    Linux)
      os_name="linux"
      ;;
    Darwin)
      os_name="darwin"
      ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
      err "Windows is not supported by scripts/install.sh. Windows users should download tspack-windows-amd64.zip from GitHub Releases."
      exit 2
      ;;
    *)
      err "Unsupported operating system: $raw_os (arch: $raw_arch)."
      exit 2
      ;;
  esac

  case "$raw_arch" in
    x86_64|amd64)
      arch_name="amd64"
      ;;
    aarch64|arm64)
      arch_name="arm64"
      ;;
    *)
      err "Unsupported CPU architecture: $raw_arch (os: $raw_os)."
      exit 2
      ;;
  esac

  artifact="tspack-${os_name}-${arch_name}.tar.gz"
}

download() {
  url="$1"
  output="$2"

  if command_exists curl; then
    curl -fsSL "$url" -o "$output"
    return
  fi

  if command_exists wget; then
    wget -q "$url" -O "$output"
    return
  fi

  err "Neither curl nor wget is available; install one and retry."
  exit 1
}

resolve_version() {
  if [ -n "$version" ]; then
    return
  fi

  latest_url="${api_base}/repos/${repo}/releases/latest"
  latest_json="$work_dir/latest.json"

  if ! download "$latest_url" "$latest_json"; then
    err "Failed to resolve the latest release from $latest_url. Set TSPACK_VERSION=vX.Y.Z and retry."
    exit 1
  fi

  version="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$latest_json" | sed -n '1p')"
  if [ -z "$version" ]; then
    err "Could not find tag_name in the latest release response. Set TSPACK_VERSION=vX.Y.Z and retry."
    exit 1
  fi
}

sha256_file() {
  file_path="$1"

  if command_exists sha256sum; then
    sha256sum "$file_path" | awk '{print $1}'
    return
  fi

  if command_exists shasum; then
    shasum -a 256 "$file_path" | awk '{print $1}'
    return
  fi

  err "No SHA256 tool found; install sha256sum or shasum and retry."
  exit 1
}

verify_checksum() {
  checksums_file="$1"
  artifact_file="$2"
  artifact_name="$3"

  if [ ! -s "$checksums_file" ]; then
    err "Missing or empty checksums.txt."
    exit 1
  fi

  expected_hash="$(awk -v name="$artifact_name" '$2 == name { print $1; found = 1; exit } END { if (!found) exit 1 }' "$checksums_file" || true)"
  if [ -z "$expected_hash" ]; then
    err "checksums.txt does not contain an entry for $artifact_name."
    exit 1
  fi

  actual_hash="$(sha256_file "$artifact_file")"
  if [ "$actual_hash" != "$expected_hash" ]; then
    err "Checksum mismatch for $artifact_name."
    err "Expected: $expected_hash"
    err "Actual:   $actual_hash"
    exit 1
  fi
}

extract_binary() {
  archive_file="$1"
  extract_dir="$2"

  mkdir -p "$extract_dir"
  tar -xzf "$archive_file" -C "$extract_dir"

  expected_path="$extract_dir/tspack-${os_name}-${arch_name}/tspack"
  if [ -f "$expected_path" ]; then
    printf '%s\n' "$expected_path"
    return
  fi

  found_path="$(find "$extract_dir" -type f -name tspack -perm -u+x | sed -n '1p')"
  if [ -n "$found_path" ]; then
    printf '%s\n' "$found_path"
    return
  fi

  found_path="$(find "$extract_dir" -type f -name tspack | sed -n '1p')"
  if [ -n "$found_path" ]; then
    printf '%s\n' "$found_path"
    return
  fi

  err "Could not find tspack binary in $artifact."
  exit 1
}

install_binary() {
  source_binary="$1"

  mkdir -p "$install_dir"

  if [ ! -d "$install_dir" ]; then
    err "Install directory does not exist and could not be created: $install_dir"
    exit 1
  fi

  if [ ! -w "$install_dir" ]; then
    err "Install directory is not writable: $install_dir"
    err "Set TSPACK_INSTALL_DIR to a user-writable directory and retry."
    exit 1
  fi

  cp "$source_binary" "$install_dir/tspack"
  chmod 0755 "$install_dir/tspack"
}

path_guidance() {
  case ":${PATH}:" in
    *":${install_dir}:"*)
      ;;
    *)
      log ""
      log "TSPack was installed to $install_dir, but that directory is not on PATH."
      if [ "$install_dir" = "${HOME}/.local/bin" ]; then
        log 'Add this to your shell profile:'
        log '  export PATH="$HOME/.local/bin:$PATH"'
      else
        log "Add this to your shell profile:"
        log "  export PATH=\"$install_dir:\$PATH\""
      fi
      ;;
  esac
}

main() {
  detect_platform

  work_dir="$(mktemp -d)"
  resolve_version

  artifact_url="${github_base}/${repo}/releases/download/${version}/${artifact}"
  checksums_url="${github_base}/${repo}/releases/download/${version}/checksums.txt"

  if [ "$dry_run" = "1" ]; then
    log "version: $version"
    log "platform: ${os_name}-${arch_name}"
    log "artifact: $artifact"
    log "artifact_url: $artifact_url"
    log "checksums_url: $checksums_url"
    log "install_dir: $install_dir"
    exit 0
  fi

  archive_file="$work_dir/$artifact"
  checksums_file="$work_dir/checksums.txt"
  extract_dir="$work_dir/extract"

  log "Installing TSPack $version for ${os_name}-${arch_name}."
  download "$artifact_url" "$archive_file"
  download "$checksums_url" "$checksums_file"
  verify_checksum "$checksums_file" "$archive_file" "$artifact"

  binary_path="$(extract_binary "$archive_file" "$extract_dir")"
  if [ ! -f "$binary_path" ]; then
    err "Extracted tspack binary is not a regular file: $binary_path"
    exit 1
  fi

  chmod 0755 "$binary_path"
  install_binary "$binary_path"

  log "Installed tspack to $install_dir/tspack"
  path_guidance
}

main
