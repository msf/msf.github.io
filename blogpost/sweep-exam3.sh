#!/usr/bin/env bash
set -euo pipefail
#
# exam_v3 sweep with per-cell logs and infrastructure tripwires.
# Stops immediately on missing artifacts / empty eval (0/0) / driver failure.
#
# Layout: results/exam_v3/{display}/seed{N}/
# Logs:   exam_v3/logs/<run-id>/
#
# Usage: ./sweep-exam3.sh [model-filter]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENDPOINT="http://localhost:8080"
TIMEOUT_DEFAULT="10m"
MAX_TOKENS="8192"
SEEDS=(42 123 456)
EXAMS=(exam_v3)
FILTER="${1:-}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
LOG_ROOT="$SCRIPT_DIR/exam_v3/logs/$RUN_ID"
CELL_LOG_DIR="$LOG_ROOT/cells"
STATUS_TSV="$LOG_ROOT/status.tsv"

mkdir -p "$CELL_LOG_DIR"
printf "ts\texam\tdisplay\tseed\tscore\tmax\ttokens\ttps\twall_s\tsummary\tcell_log\n" > "$STATUS_TSV"

declare -A SWAP_NAME=(
  [gemma4-26b-mxfp4]="gemma4-26b-mxfp4-64k"
  [qwen35-35b-mxfp4]="qwen35-35b-mxfp4"
  [qwen36-35b-q5km]="qwen36-35b-q5km-thinkoff"
  [gpt-oss]="gpt-oss"
  [qwen3-coder-30b-draft]="qwen3-coder-draft"
  [gemma4-e4b-q8]="gemma4-e4b"
  [qwen35-9b-q4km]="qwen35-9b"
  [gemma4-26b-q8]="gemma4-26b-q8-32k"
  [qwen35-35b-q6k]="qwen35-35b-q6k"
)

declare -A TIMEOUT_OVERRIDE=(
  [qwen3-coder-30b-draft]="15m"
)

declare -A SKIP_REASON=(
  [qwen3-coder-30b-draft]="blocked: local draft config still times out at 15m/8192 after startup fix"
)

MODEL_ORDER=(
  gemma4-26b-mxfp4
  qwen35-35b-mxfp4
  qwen36-35b-q5km
  gpt-oss
  qwen3-coder-30b-draft
  gemma4-e4b-q8
  qwen35-9b-q4km
  gemma4-26b-q8
  qwen35-35b-q6k
)

unload_all() {
  curl -fsS -X POST "$ENDPOINT/api/models/unload" > /dev/null 2>&1 || true
  sleep 2
}

tripwire() {
  local message="$1"
  echo "TRIPWIRE: $message" | tee -a "$LOG_ROOT/alerts.log" >&2
  unload_all
  exit 1
}

summarize_result_json() {
  local result_json="$1"
  python3 - "$result_json" <<'PY'
import json, sys
p = sys.argv[1]
d = json.load(open(p))
e = d.get("eval", {})
summary = (e.get("summary") or "").replace("\n", " ").replace("\t", " ")
print("\t".join([
    str(e.get("score", "")),
    str(e.get("max", "")),
    str(d.get("tokens", "")),
    str(d.get("tps", "")),
    str(d.get("wall_s", "")),
    summary,
]))
PY
}

run_one() {
  local exam="$1" display="$2" swap="$3" seed="$4"
  local timeout="${TIMEOUT_OVERRIDE[$display]:-$TIMEOUT_DEFAULT}"
  local outdir="$SCRIPT_DIR/results/${exam}/${display}/seed${seed}"
  local celllog="$CELL_LOG_DIR/${display}-seed${seed}.log"
  mkdir -p "$outdir"

  if [ -f "$outdir/result.json" ]; then
    local summary_line
    summary_line=$(summarize_result_json "$outdir/result.json")
    IFS=$'\t' read -r score max tokens tps wall summary <<< "$summary_line"
    echo "  $exam seed$seed: done ($score/$max)"
    printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
      "$(date -Is)" "$exam" "$display" "$seed" "$score" "$max" "$tokens" "$tps" "$wall" "$summary" "$celllog" >> "$STATUS_TSV"
    return
  fi

  local tmpout model_tmp driver_rc result_json summary_line score max tokens tps wall summary
  tmpout=$(mktemp -d)
  model_tmp="$tmpout/$swap"

  set +e
  go run /home/miguel/play/llama/exam-driver.go \
    -endpoint "$ENDPOINT" \
    -prompt "$SCRIPT_DIR/${exam}/prompt.txt" \
    -eval "$SCRIPT_DIR/${exam}/eval.sh" \
    -out "$tmpout" \
    -timeout "$timeout" \
    -seed "$seed" \
    -max-tokens "$MAX_TOKENS" \
    "$swap" > "$celllog" 2>&1
  driver_rc=$?
  set -e

  if [ -d "$model_tmp" ]; then
    cp -a "$model_tmp"/* "$outdir/" 2>/dev/null || true
  fi
  rm -rf "$tmpout"

  result_json="$outdir/result.json"
  if [ $driver_rc -ne 0 ]; then
    tripwire "$display seed$seed driver exited rc=$driver_rc; see $celllog"
  fi
  if [ ! -f "$result_json" ]; then
    tripwire "$display seed$seed produced no result.json; see $celllog"
  fi

  summary_line=$(summarize_result_json "$result_json")
  IFS=$'\t' read -r score max tokens tps wall summary <<< "$summary_line"
  printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
    "$(date -Is)" "$exam" "$display" "$seed" "$score" "$max" "$tokens" "$tps" "$wall" "$summary" "$celllog" >> "$STATUS_TSV"

  if [ -z "$tokens" ] || [ "$tokens" = "0" ]; then
    tripwire "$display seed$seed returned zero tokens; see $celllog"
  fi
  if [ -z "$max" ] || [ "$max" = "0" ]; then
    tripwire "$display seed$seed produced empty eval denominator ($score/$max); see $celllog"
  fi

  echo "  $exam seed$seed: $score/$max (${summary:-ok})"
}

if ! curl -fsS "$ENDPOINT/health" > /dev/null 2>&1; then
  echo "ERROR: llama-swap not responding at $ENDPOINT" >&2
  exit 1
fi

trap unload_all EXIT INT TERM
unload_all

for display in "${MODEL_ORDER[@]}"; do
  [[ -n "$FILTER" ]] && ! echo "$display" | grep -qE "$FILTER" && continue

  if [[ -n "${SKIP_REASON[$display]:-}" ]]; then
    echo "===== $display SKIPPED: ${SKIP_REASON[$display]} =====" | tee -a "$LOG_ROOT/run.log"
    printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
      "$(date -Is)" "exam_v3" "$display" "-" "" "" "" "" "" "SKIPPED: ${SKIP_REASON[$display]}" "" >> "$STATUS_TSV"
    continue
  fi

  swap="${SWAP_NAME[$display]}"
  echo ""
  timeout="${TIMEOUT_OVERRIDE[$display]:-$TIMEOUT_DEFAULT}"
  echo "===== $display (swap: $swap, max_tokens: $MAX_TOKENS, timeout: $timeout, run_id: $RUN_ID) =====" | tee -a "$LOG_ROOT/run.log"

  for exam in "${EXAMS[@]}"; do
    for seed in "${SEEDS[@]}"; do
      run_one "$exam" "$display" "$swap" "$seed"
    done
  done

  unload_all
done

echo "" | tee -a "$LOG_ROOT/run.log"
echo "===== exam_v3 SWEEP COMPLETE =====" | tee -a "$LOG_ROOT/run.log"
