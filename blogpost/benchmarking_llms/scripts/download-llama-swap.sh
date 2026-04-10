#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/play/llama}"
REPO="mostlygeek/llama-swap"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { printf 'missing: %s\n' "$1" >&2; exit 1; }
}

need_cmd curl
need_cmd jq
need_cmd tar

release_json="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"
tag="$(jq -r '.tag_name // empty' <<< "$release_json")"
version="${tag#v}"

if [[ -z "$tag" ]]; then
  printf 'failed to get latest llama-swap release\n' >&2
  exit 1
fi

asset_name="llama-swap_${version}_linux_amd64.tar.gz"
download_url="$(jq -r --arg name "$asset_name" \
  'first(.assets[] | select(.name == $name) | .browser_download_url) // empty' \
  <<< "$release_json")"

dest="${INSTALL_DIR}/llama-swap"

if [[ -x "$dest" ]]; then
  current="$("$dest" -version 2>/dev/null || true)"
  if [[ "$current" == *"$version"* ]]; then
    printf 'already installed: llama-swap %s\n' "$tag"
    exit 0
  fi
fi

if [[ -z "$download_url" ]]; then
  printf 'asset %s not found in release %s\n' "$asset_name" "$tag" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap "rm -rf '$tmp'" EXIT

printf 'downloading llama-swap %s\n' "$tag"
curl -fsSL -o "${tmp}/${asset_name}" "$download_url"
tar -xzf "${tmp}/${asset_name}" -C "$tmp"

if [[ ! -f "${tmp}/llama-swap" ]]; then
  printf 'archive did not contain llama-swap binary\n' >&2
  exit 1
fi

chmod +x "${tmp}/llama-swap"
mv "${tmp}/llama-swap" "$dest"

printf 'installed: %s\n' "$dest"
