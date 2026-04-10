#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME/play/llama}"
REPO="${REPO:-ggml-org/llama.cpp}"
RELEASE_API="https://api.github.com/repos/${REPO}/releases/latest"
SHOW_PROGRESS="${SHOW_PROGRESS:-0}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd jq
need_cmd tar
need_cmd mktemp

mkdir -p "$ROOT_DIR"

release_json="$(curl -fsSL "$RELEASE_API")"
tag="$(jq -r '.tag_name // empty' <<< "$release_json")"

if [[ -z "$tag" ]]; then
  printf 'failed to read latest llama.cpp tag from %s\n' "$RELEASE_API" >&2
  exit 1
fi

asset_name="llama-${tag}-bin-ubuntu-vulkan-x64.tar.gz"
download_url="$(jq -r --arg name "$asset_name" 'first(.assets[] | select(.name == $name) | .browser_download_url) // empty' <<< "$release_json")"
dest_dir="${ROOT_DIR}/llama-${tag}"

if [[ -x "${dest_dir}/llama-cli" ]]; then
  printf 'already installed: %s\n' "$dest_dir"
  exit 0
fi

if [[ -e "$dest_dir" ]]; then
  printf 'target exists but is not a complete llama.cpp install: %s\n' "$dest_dir" >&2
  exit 1
fi

if [[ -z "$download_url" ]]; then
  printf 'could not find release asset %s in %s\n' "$asset_name" "$RELEASE_API" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
archive_path="${tmp_dir}/${asset_name}"
staging_dir="${tmp_dir}/llama-${tag}"

cleanup() {
  rm -rf "$tmp_dir"
}

trap cleanup EXIT

printf 'downloading %s\n' "$asset_name"
if [[ "$SHOW_PROGRESS" == "1" ]]; then
  curl -fL --progress-bar -o "$archive_path" "$download_url"
else
  curl -fsSL -o "$archive_path" "$download_url"
fi

tar -xzf "$archive_path" -C "$tmp_dir"

if [[ ! -x "${staging_dir}/llama-cli" ]]; then
  printf 'downloaded archive did not contain llama-cli in %s\n' "$staging_dir" >&2
  exit 1
fi

mv "$staging_dir" "$dest_dir"
ln -sfn "$dest_dir" "${ROOT_DIR}/llama-current"

printf 'installed: %s\n' "$dest_dir"
printf 'llama-cli: %s\n' "${dest_dir}/llama-cli"
printf 'symlink:   %s -> %s\n' "${ROOT_DIR}/llama-current" "$dest_dir"
