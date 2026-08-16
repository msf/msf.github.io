#!/usr/bin/env bash
# One-shot: exam_v3 (5 seeds) then exam_v4 (strat20) for qwen38-27b, with MTP.
# Written 2026-08-14 after the first pass ran with --spec-type unset.
set -euo pipefail

LAB=$(cd "$(dirname "$0")/.." && pwd)
cd "$LAB"

TS=$(date +%Y%m%d-%H%M%S)
LOGDIR="$LAB/artifacts/logs/qwen38-suite"
mkdir -p "$LOGDIR"

echo "==> exam_v3 start $(date -Is)"
SEEDS="42 123 456 789 1011" CELLS="K" TEMP=1.0 MAX_TOKENS=16384 \
  RESULTS_DIR="$LAB/artifacts/results/exam_v3_r9700_qwen38_mtp" \
  ./scripts/sweep-exam3-rocmfp4.sh > "$LOGDIR/exam_v3-$TS.log" 2>&1 \
  || echo "!! exam_v3 exited $? — continuing to exam_v4"
echo "==> exam_v3 done $(date -Is)"

echo "==> exam_v4 start $(date -Is)"
TERM=dumb NO_COLOR=1 ./scripts/tb21-run.sh qwen38-27b strat20 > "$LOGDIR/exam_v4-$TS.log" 2>&1 \
  || echo "!! exam_v4 exited $?"
echo "==> exam_v4 done $(date -Is)"
