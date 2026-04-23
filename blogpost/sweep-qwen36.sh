#!/usr/bin/env bash
set -euo pipefail
#
# One-shot sweep for the Qwen3.6 Phase 1 evaluation.
# Variants:
#   qwen36-35b-q5km-thinkoff  -> max_tokens 8192  (no thinking, exam v2 needs ~5k output)
#   qwen36-35b-q5km-thinkon   -> max_tokens 16384 (thinking burns ~8k before output)
#   gemma4-26b-mxfp4-32k      -> max_tokens 8192  (no thinking, 32k ctx version)
#
# Writes to: results/{exam}/{display}/seed{N}/
#
# Safety: if result.json already exists for a cell, skip (same behavior as sweep.sh).

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENDPOINT="http://localhost:8080"
TIMEOUT="12m"   # slightly larger than sweep.sh default (10m) to give thinking runs room
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

run_one() {
  local exam="$1" display="$2" swap="$3" seed="$4" maxtok="$5"
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
    -max-tokens "$maxtok" \
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
  swap="${SWAP_NAME[$display]}"
  maxtok="${MAX_TOKENS[$display]}"
  echo ""
  echo "===== $display (swap: $swap, max_tokens: $maxtok) ====="

  for exam in "${EXAMS[@]}"; do
    for seed in "${SEEDS[@]}"; do
      run_one "$exam" "$display" "$swap" "$seed" "$maxtok"
    done
  done

  # Unload after this model is fully done
  curl -s "$ENDPOINT/models/unload" -X POST > /dev/null 2>&1
  sleep 2
done

echo ""
echo "===== Qwen3.6 SWEEP COMPLETE ====="
