#!/usr/bin/env bash
set -euo pipefail
#
# Multi-seed sweep. For each model: run ALL exams × ALL seeds before switching.
# Model stays loaded → warm KV cache, no redundant swaps.
#
# Layout: results/{exam}/{model}/seed{N}/
#
# Usage: ./sweep.sh [model-filter]   # regex filter on display name, empty = all

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENDPOINT="http://localhost:8080"
TIMEOUT="10m"
SEEDS=(42 123 456)
EXAMS=(exam_v1 exam_v2)
FILTER="${1:-}"

# Display name → llama-swap model name
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

# Priority order
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

run_one() {
  local exam="$1" display="$2" swap="$3" seed="$4"
  local outdir="$SCRIPT_DIR/results/${exam}/${display}/seed${seed}"
  mkdir -p "$outdir"

  if [ -f "$outdir/result.json" ]; then
    local score
    score=$(python3 -c "import json; d=json.load(open('$outdir/result.json')); print(f\"{d['eval']['score']}/{d['eval']['max']}\")" 2>/dev/null || echo "?")
    echo "  $exam seed$seed: done ($score)"
    return
  fi

  local tmpout
  tmpout=$(mktemp -d)
  go run /home/miguel/play/llama/exam-driver.go \
    -endpoint "$ENDPOINT" \
    -prompt "$SCRIPT_DIR/${exam}/prompt.txt" \
    -eval "$SCRIPT_DIR/${exam}/eval.sh" \
    -out "$tmpout" \
    -timeout "$TIMEOUT" \
    -seed "$seed" \
    "$swap" 2>&1 | tail -1

  if [ -d "$tmpout/$swap" ]; then
    cp -a "$tmpout/$swap"/* "$outdir/" 2>/dev/null || true
  fi
  rm -rf "$tmpout"
}

if ! curl -s "$ENDPOINT/health" > /dev/null 2>&1; then
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

  # Unload after this model is fully done
  curl -s "$ENDPOINT/models/unload" -X POST > /dev/null 2>&1
  sleep 1
done

echo ""
echo "===== SWEEP COMPLETE ====="
