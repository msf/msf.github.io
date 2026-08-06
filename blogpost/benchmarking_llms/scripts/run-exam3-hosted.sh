#!/usr/bin/env bash
set -uo pipefail
#
# exam_v3 against a hosted OpenAI-compatible endpoint (Anthropic, OpenAI).
# Same prompt, evaluator and driver as the local sweep — only the endpoint and
# the bearer token differ.
#
# Requires EXAM_API_KEY in the environment. It is never echoed or logged.
#
#   EXAM_API_KEY=... ./run-exam3-hosted.sh
#   TEMPS=0.6 SEEDS=42 MODEL=gpt-5 ENDPOINT=https://api.openai.com ./run-exam3-hosted.sh
#
# Hosted APIs do not return llama.cpp's `timings`, so tps is always 0 here.
# score and wall_s stay valid.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR="$LAB_ROOT/bench/exam_v3"
DRIVER="$LAB_ROOT/exam-driver.go"

MODEL="${MODEL:-claude-haiku-4-5-20251001}"
DISPLAY_NAME="${DISPLAY_NAME:-haiku-45}"
ENDPOINT="${ENDPOINT:-https://api.anthropic.com}"
SEEDS="${SEEDS:-42 123}"
TEMPS="${TEMPS:-0.6 1.0}"
MAX_TOKENS="${MAX_TOKENS:-16384}"
ATTEMPT_TIMEOUT="${ATTEMPT_TIMEOUT:-15m}"

RESULTS_DIR="${RESULTS_DIR:-$LAB_ROOT/artifacts/results/exam_v3_hosted}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)-hosted}"
LOG_ROOT="${LOG_ROOT:-$LAB_ROOT/artifacts/logs/exam_v3/$RUN_ID}"
STATUS_TSV="$LOG_ROOT/status.tsv"

mkdir -p "$LOG_ROOT" "$RESULTS_DIR"
log()  { printf '%s  %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$LOG_ROOT/sweep.log" >&2; }
fail() { log "ABORT: $*"; exit 1; }

[ -n "${EXAM_API_KEY:-}" ] || fail "EXAM_API_KEY not set"
[ -f "$BENCH_DIR/prompt.txt" ] || fail "missing prompt"
command -v jq >/dev/null || fail "jq missing"

# Same gate as the local sweep: a perfect answer must score 13/13, else no
# model score from this run means anything.
probe=$(mktemp -d)
python3 "$BENCH_DIR/make-reference-response.py" "$BENCH_DIR" "$probe/response.txt" \
  2>"$LOG_ROOT/preflight-reference.log"
score=$(bash "$BENCH_DIR/eval.sh" "$probe/response.txt" "$probe" 2>"$LOG_ROOT/preflight-grader.log" \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); print("%d/%d"%(d["score"],d["max"]))')
rm -rf "$probe"
[ "$score" = "13/13" ] || fail "evaluator scored reference $score, expected 13/13"
log "preflight: evaluator scores reference solution 13/13"

go build -o /dev/null "$DRIVER" || fail "driver does not build"

printf 'ts\tdisplay\tmodel\ttemp\tseed\tscore\tmax\ttokens\twall_s\tsummary\n' >"$STATUS_TSV"
{
  echo "run_id:     $RUN_ID"
  echo "endpoint:   $ENDPOINT"
  echo "model:      $MODEL"
  echo "temps:      $TEMPS"
  echo "seeds:      $SEEDS"
  echo "max_tokens: $MAX_TOKENS"
  echo "note:       tps is 0 for hosted endpoints (no llama.cpp timings block)"
} | tee "$LOG_ROOT/run-info.txt" >&2

for temp in $TEMPS; do
  for seed in $SEEDS; do
    outdir="$RESULTS_DIR/${DISPLAY_NAME}-t${temp}/seed${seed}"
    celllog="$LOG_ROOT/${DISPLAY_NAME}-t${temp}-seed${seed}.log"
    mkdir -p "$outdir"
    if [ -f "$outdir/result.json" ]; then
      log "  t$temp seed$seed: already present, skipping"
    else
      log "  t$temp seed$seed: running"
      tmpout=$(mktemp -d)
      go run "$DRIVER" \
        -endpoint "$ENDPOINT" \
        -prompt "$BENCH_DIR/prompt.txt" \
        -eval "$BENCH_DIR/eval.sh" \
        -out "$tmpout" \
        -seed "$seed" -temp "$temp" \
        -max-tokens "$MAX_TOKENS" -timeout "$ATTEMPT_TIMEOUT" \
        "$MODEL" >"$celllog" 2>&1
      rc=$?
      [ -d "$tmpout/$MODEL" ] && cp -a "$tmpout/$MODEL"/. "$outdir/" 2>/dev/null
      rm -rf "$tmpout"
      if [ ! -f "$outdir/result.json" ]; then
        log "  TRIPWIRE t$temp seed$seed: no result.json (rc=$rc); see $celllog"
        continue
      fi
    fi
    python3 - "$DISPLAY_NAME" "$MODEL" "$temp" "$seed" "$outdir/result.json" >>"$STATUS_TSV" <<'PY'
import json, sys, datetime
display, model, temp, seed, path = sys.argv[1:6]
d = json.load(open(path)); e = d.get("eval", {})
print("\t".join(str(x) for x in [
    datetime.datetime.now().isoformat(timespec="seconds"),
    display, model, temp, seed,
    e.get("score",""), e.get("max",""), d.get("tokens",""),
    round(d.get("wall_s") or 0, 1),
    (e.get("summary") or "").replace("\n"," ").replace("\t"," "),
]))
PY
    tail -1 "$STATUS_TSV" | awk -F'\t' '{printf "  -> %s/%s  %s tok  %ss\n",$6,$7,$8,$9}' >&2
  done
done

log "hosted run finished; status: $STATUS_TSV"
cut -f2-9 "$STATUS_TSV" | column -t -s$'\t' >&2
