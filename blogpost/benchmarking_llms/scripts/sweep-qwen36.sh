#!/usr/bin/env bash
set -euo pipefail
#
# One-shot sweep for the Qwen3.6 phase-1 evaluation.
# Writes to: artifacts/results/{exam}/{display}/seed{N}/

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR="$LAB_ROOT/bench"
RESULTS_DIR="${RESULTS_DIR:-$LAB_ROOT/artifacts/results}"
DRIVER="${DRIVER:-$LAB_ROOT/exam-driver.go}"
ENDPOINT="${ENDPOINT:-http://localhost:8080}"
TIMEOUT="${TIMEOUT:-12m}"
SEEDS=(42 123 456)
EXAMS=(exam_v1 exam_v2)

# display-name -> (swap-name, max-tokens)
declare -A SWAP_NAME=(
  [qwen36-35b-q5km-thinkoff]="qwen36-35b-q5km-thinkoff"
  [qwen36-35b-q5km-thinkon]="qwen36-35b-q5km-thinkon"
  [gemma4-26b-mxfp4-32k]="gemma4-26b-mxfp4-32k"
)
declare -A MAX_TOKENS=(
  [qwen36-35b-q5km-thinkoff]="8192"
  [qwen36-35b-q5km-thinkon]="16384"
  [gemma4-26b-mxfp4-32k]="8192"
)

MODEL_ORDER=(
  qwen36-35b-q5km-thinkoff
  gemma4-26b-mxfp4-32k
  qwen36-35b-q5km-thinkon
)

unload_all() {
  curl -fsS -X POST "$ENDPOINT/api/models/unload" > /dev/null 2>&1 && return 0
  curl -fsS -X POST "$ENDPOINT/models/unload" > /dev/null 2>&1 && return 0
  return 1
}

run_one() {
  local exam="$1" display="$2" swap="$3" seed="$4" maxtok="$5"
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
    -max-tokens "$maxtok" \
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
  swap="${SWAP_NAME[$display]}"
  maxtok="${MAX_TOKENS[$display]}"
  echo ""
  echo "===== $display (swap: $swap, max_tokens: $maxtok) ====="

  for exam in "${EXAMS[@]}"; do
    for seed in "${SEEDS[@]}"; do
      run_one "$exam" "$display" "$swap" "$seed" "$maxtok"
    done
  done

  unload_all || true
  sleep 2
done

echo ""
echo "===== Qwen3.6 SWEEP COMPLETE ====="
