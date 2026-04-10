#!/usr/bin/env bash
set -euo pipefail

# Framework 13 bootstrap / preflight check.
# Intentionally check-first: validates the local runtime/layout and reports
# drift against the known-good machine plan. It does not mutate local state.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="$LAB_ROOT/machines/framework13.env"

if [ ! -f "$ENV_FILE" ]; then
  printf 'missing env file: %s\n' "$ENV_FILE" >&2
  exit 1
fi

# shellcheck disable=SC1090
. "$ENV_FILE"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

need_cmd python3
need_cmd readlink
need_cmd hostname

critical=0
warn=0

ok()   { printf 'OK      %s\n' "$1"; }
warnf(){ printf 'WARN    %s\n' "$1"; warn=$((warn + 1)); }
failf(){ printf 'FAIL    %s\n' "$1"; critical=$((critical + 1)); }

check_path() {
  local label="$1" path="$2"
  if [ -e "$path" ]; then
    ok "$label: $path"
  else
    failf "$label missing: $path"
  fi
}

http_ok() {
  python3 - "$1" <<'PY'
import sys, urllib.request
url = sys.argv[1]
try:
    with urllib.request.urlopen(url, timeout=2) as r:
        code = getattr(r, 'status', 200)
        if 200 <= code < 300:
            raise SystemExit(0)
except Exception:
    raise SystemExit(1)
PY
}

printf '== framework13 bootstrap / preflight ==\n'
printf 'machine: %s\n' "$MACHINE_LABEL"
printf 'lab:     %s\n' "$LAB_ROOT"
printf 'runtime: %s\n' "$LLAMA_HOME"
printf '\n'

current_host="$(hostname 2>/dev/null || echo unknown)"
if [ "$current_host" = "$MACHINE_HOSTNAME" ]; then
  ok "hostname: $current_host"
else
  warnf "hostname mismatch: expected $MACHINE_HOSTNAME, got $current_host"
fi

current_os="$(sed -n 's/^PRETTY_NAME="\(.*\)"/\1/p' /etc/os-release | head -1)"
if [ -n "$current_os" ] && [ "$current_os" = "$MACHINE_OS" ]; then
  ok "os: $current_os"
elif [ -n "$current_os" ]; then
  warnf "os drift: expected $MACHINE_OS, got $current_os"
else
  warnf "could not read PRETTY_NAME from /etc/os-release"
fi

if command -v powerprofilesctl >/dev/null 2>&1; then
  current_profile="$(powerprofilesctl get 2>/dev/null || true)"
  if [ -n "$current_profile" ]; then
    if [ "$current_profile" = "$BENCHMARK_POWER_PROFILE" ]; then
      ok "power profile: $current_profile"
    else
      warnf "power profile: expected $BENCHMARK_POWER_PROFILE for reproducible benchmarking, got $current_profile"
    fi
  else
    warnf "power profile: powerprofilesctl present but returned no value"
  fi
else
  warnf "power profile: powerprofilesctl not installed; cannot verify $BENCHMARK_POWER_PROFILE"
fi

printf '\n== runtime paths ==\n'
check_path "lab root" "$LAB_ROOT"
check_path "bench dir" "$BENCH_DIR"
check_path "results dir" "$RESULTS_DIR"
check_path "logs dir" "$LOGS_DIR"
check_path "llama home" "$LLAMA_HOME"
check_path "llama-swap" "$LLAMA_SWAP"
check_path "llama-swap config" "$LLAMA_SWAP_CONFIG"
check_path "HF hub root" "$HF_HUB_ROOT"

current_release_target="$(readlink -f "$LLAMA_CURRENT" 2>/dev/null || true)"
if [ -z "$current_release_target" ]; then
  failf "llama-current target unreadable: $LLAMA_CURRENT"
else
  current_release_name="$(basename "$current_release_target")"
  if [ "$current_release_name" = "$LLAMA_RELEASE" ]; then
    ok "llama-current: $current_release_target"
  else
    warnf "llama-current drift: expected $LLAMA_RELEASE, got $current_release_name ($current_release_target)"
  fi
fi

if http_ok "$LLAMA_SWAP_ENDPOINT/health"; then
  ok "llama-swap endpoint: $LLAMA_SWAP_ENDPOINT is responding"
else
  warnf "llama-swap endpoint: $LLAMA_SWAP_ENDPOINT is not responding"
fi

printf '\n== unsloth snapshot freshness ==\n'
snapshot_output="$(python3 - "$LLAMA_SWAP_CONFIG" "$HF_HUB_ROOT" <<'PY'
import json, os, re, sys, urllib.request
cfg_path, hub_root = sys.argv[1:3]
cfg = open(cfg_path, 'r', encoding='utf-8').read()
repos = [
    ('qwen36', 'unsloth/Qwen3.6-35B-A3B-GGUF'),
    ('gemma4-26b', 'unsloth/gemma-4-26B-A4B-it-GGUF'),
    ('gemma4-e4b', 'unsloth/gemma-4-E4B-it-GGUF'),
]

def local_sha(repo: str) -> str:
    slug = 'models--' + repo.replace('/', '--')
    m = re.search(rf'{re.escape(slug)}/snapshots/([0-9a-f]+)/', cfg)
    return m.group(1) if m else ''

for _label, repo in repos:
    local = local_sha(repo)
    remote = ''
    status = 'UNKNOWN'
    cached = 'no'
    error = ''
    try:
        with urllib.request.urlopen('https://huggingface.co/api/models/' + repo, timeout=10) as r:
            remote = json.load(r).get('sha', '')
    except Exception as exc:
        error = str(exc)

    if remote:
        cached = 'yes' if os.path.isdir(os.path.join(hub_root, 'models--' + repo.replace('/', '--'), 'snapshots', remote)) else 'no'
        if local == remote:
            status = 'CURRENT'
        elif local:
            status = 'STALE'
        else:
            status = 'MISSING'

    if error:
        print(f'WARN    {repo}: remote lookup failed ({error})')
    else:
        line = f'{repo}: local={local or "MISSING"} remote={remote or "UNKNOWN"} cached_remote={cached}'
        if status == 'CURRENT':
            print(f'OK      {line}')
        elif status in {'STALE', 'MISSING'}:
            print(f'WARN    {line} status={status}')
        else:
            print(f'WARN    {line} status=UNKNOWN')
PY
)"
printf '%s\n' "$snapshot_output"
snapshot_warns="$(printf '%s\n' "$snapshot_output" | grep -c '^WARN    ' || true)"
warn=$((warn + snapshot_warns))

printf '\n== next actions ==\n'
printf '%s\n' "- If runtime is missing on a fresh host: review $LAB_ROOT/scripts/download-llama.sh and $LAB_ROOT/scripts/download-llama-swap.sh first."
printf '%s\n' "- If Qwen3.6 or Gemma4-26B is stale: refresh the repo in the HF cache, then update explicit snapshot paths in $LLAMA_SWAP_CONFIG."
printf '%s\n' "- After any snapshot refresh: smoke-test qwen36-35b-q5km-thinkoff, qwen36-35b-q5km-thinkon, and gemma4-26b-mxfp4-64k before running a real sweep."
printf '%s\n' "- Use $LAB_ROOT/machines/framework13.md as the interpretation guide for this box."

printf '\nsummary: %d critical, %d warnings\n' "$critical" "$warn"

if [ "$critical" -ne 0 ]; then
  exit 1
fi
