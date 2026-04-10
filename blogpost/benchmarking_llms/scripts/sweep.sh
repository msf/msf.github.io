#!/usr/bin/env bash
set -euo pipefail
#
# Historical multi-seed sweep for exam_v1 + exam_v2.
# For each model: run all exams × all seeds before switching.
#
# Current canonical layout:
#   bench/                prompts + evaluators
#   artifacts/results/    sweep outputs
#   exam-driver.go        llama-swap driver
#
# Usage: ./sweep.sh [model-filter]
# Notes:
# - This script preserves the historical display names used in the published
#   results directories.
# - Verify your local llama-swap config still defines the mapped swap keys
#   before re-running old comparisons.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR="$LAB_ROOT/bench"
RESULTS_DIR="${RESULTS_DIR:-$LAB_ROOT/artifacts/results}"
DRIVER="${DRIVER:-$LAB_ROOT/exam-driver.go}"
ENDPOINT="${ENDPOINT:-http://localhost:8080}"
TIMEOUT="${TIMEOUT:-10m}"
SEEDS=(42 123 456)
EXAMS=(exam_v1 exam_v2)
FILTER="${1:-}"

# Display name -> llama-swap model key.
declare -A SWAP_NAME=(
  [qwen35-35b-q4km]="qwen35-35b"
  [qwen35-35b-q5km]="qwen35-35b-q5km"
  [qwen35-35b-q6k]="qwen35-35b-q6k"
  [qwen35-35b-mxfp4]="qwen35-35b-mxfp4"
  [qwen3-coder-30b-draft]="qwen3-coder-draft"
  [gemma4-26b-q4km]="gemma4"
  [gemma4-26b-mxfp4]="gemma4-mxfp4"
  [gemma4-26b-q5km]="gemma4-q5km"
  [gpt-oss-20b]="gpt-oss"
  [qwen35-9b-q4km]="qwen35-9b"
  [gemma4-e4b-q8]="gemma4-e4b"
)

MODEL_ORDER=(
  qwen35-35b-q5km
  gemma4-26b-q5km
  gemma4-26b-mxfp4
  gemma4-26b-q4km
  qwen35-35b-q4km
  qwen35-35b-q6k
  qwen35-35b-mxfp4
  qwen3-coder-30b-draft
  gpt-oss-20b
  qwen35-9b-q4km
  gemma4-e4b-q8
)

unload_all() {
  curl -fsS -X POST "$ENDPOINT/api/models/unload" > /dev/null 2>&1 && return 0
  curl -fsS -X POST "$ENDPOINT/models/unload" > /dev/null 2>&1 && return 0
  return 1
}

run_one() {
  local exam="$1" display="$2" swap="$3" seed="$4"
  local outdir="$RESULTS_DIR/${exam}/${display}/seed${seed}"
  mkdir -p "$outdir"

  if [ -f "$outdir/result.json" ]; then
    local score
    score=$(python3 -c "import json; d=json.load(open('$outdir/result.json')); print(f\"{d['eval']['score']}/{d['eval']['max']}\")" 2>/dev/null || echo "?")
    echo "  $exam seed$seed: done ($score)"
    return
  fi

  local tmpout
  tmpout=$(mktemp -d)
  go run "$DRIVER" \
    -endpoint "$ENDPOINT" \
    -prompt "$BENCH_DIR/${exam}/prompt.txt" \
    -eval "$BENCH_DIR/${exam}/eval.sh" \
    -out "$tmpout" \
    -timeout "$TIMEOUT" \
    -seed "$seed" \
    "$swap" 2>&1 | tail -1

  if [ -d "$tmpout/$swap" ]; then
    cp -a "$tmpout/$swap"/* "$outdir/" 2>/dev/null || true
  fi
  rm -rf "$tmpout"
}

if [ ! -f "$DRIVER" ]; then
  echo "ERROR: exam driver not found: $DRIVER" >&2
  exit 1
fi

if ! curl -fsS "$ENDPOINT/health" > /dev/null 2>&1; then
  echo "ERROR: llama-swap not responding at $ENDPOINT" >&2
  exit 1
fi

for display in "${MODEL_ORDER[@]}"; do
  [[ -n "$FILTER" ]] && ! echo "$display" | grep -qE "$FILTER" && continue

  swap="${SWAP_NAME[$display]}"
  echo ""
  echo "===== $display (swap: $swap) ====="

  for exam in "${EXAMS[@]}"; do
    for seed in "${SEEDS[@]}"; do
      run_one "$exam" "$display" "$swap" "$seed"
    done
  done

  unload_all || true
  sleep 1
done

echo ""
echo "===== SWEEP COMPLETE ====="
