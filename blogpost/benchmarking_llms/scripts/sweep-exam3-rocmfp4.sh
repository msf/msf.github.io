#!/usr/bin/env bash
set -uo pipefail
#
# exam_v3 one-shot sweep: ROCmFP4 vs Unsloth vs Gemma, on this laptop.
#
#   cell A  rocmfp4-moe-35b-a3b   ROCmFP4 ACE-SABER 19.0 GB, HIP, container :18080
#   cell B  qwen36-moe-unsloth    Unsloth UD-Q4_K_XL 22.9 GB, Vulkan, llama-swap :8090
#   cell C  gemma4-26b-qat        Gemma 4 26B-A4B QAT 14.2 GB, Vulkan, llama-swap :8090
#
# A vs B is the question: same model family, same MTP, different quant+tune.
# C is the quality reference — it topped the April exam_v3 table at 11/13.
#
# Uses the committed exam_v3 prompt, evaluator and driver unchanged. No new
# harness. Scores are /13 from `go test -race -json` with a fixed denominator.
#
# Usage:
#   ./sweep-exam3-rocmfp4.sh                      # 3 cells x seeds 42 123
#   CELLS=A SEEDS=42 ./sweep-exam3-rocmfp4.sh     # one cell, one seed
#
# Idempotent: a cell/seed with an existing result.json is skipped, so an
# interrupted sweep resumes.
#
# --- Deviations from the April 2026 clean rerun, read before comparing ---
#
# 1. Reasoning is ON here; the April run used `--reasoning off`. The current
#    llama-swap entries are the reasoning-enabled ones and that is how the box
#    actually serves. `*-nothink` aliases exist if you want the old mode.
# 2. Gemma is the QAT rebuild (gemma-4-26B-A4B-it-qat-UD-Q4_K_XL, 14.2 GB), not
#    April's MXFP4 build. Same family, different weights — that is why it is
#    being re-run rather than quoted from the old table.
# 3. Serving stack moved (llama.cpp release, llama-swap v241, MTP drafters).
#
# So: treat the April numbers as historical context, not as a control. Cell C
# is the control for this run.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BENCH_DIR="$LAB_ROOT/bench/exam_v3"
DRIVER="$LAB_ROOT/exam-driver.go"
RESULTS_DIR="${RESULTS_DIR:-$LAB_ROOT/artifacts/results/exam_v3}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
LOG_ROOT="${LOG_ROOT:-$LAB_ROOT/artifacts/logs/exam_v3/$RUN_ID}"
STATUS_TSV="$LOG_ROOT/status.tsv"

SEEDS="${SEEDS:-42 123}"
CELLS="${CELLS:-A B C}"

# max_tokens MUST exceed --reasoning-budget (8192). The April rerun used 8192
# with `--reasoning off`; with reasoning ON, 8192 lets thinking consume the
# entire allowance and `content` comes back empty -> a structural 0/13. Measured
# 2026-08-06: at 8192 the ROCmFP4 cell returned 8192 tokens, all of them
# reasoning_content, empty content. At 16384 the same server returns
# finish=stop with both reasoning and content.
MAX_TOKENS="${MAX_TOKENS:-16384}"
TEMP="${TEMP:-1.0}"
# 16384 tok at the ROCmFP4 cell's ~13 t/s is ~21 min, so 15m would truncate.
ATTEMPT_TIMEOUT="${ATTEMPT_TIMEOUT:-30m}"

# Hard stop for the whole sweep. Partial results are kept.
SWEEP_BUDGET_S="${SWEEP_BUDGET_S:-21600}"   # 6h: 6 attempts x up to 30m + loads
SWEEP_DEADLINE=$(( $(date +%s) + SWEEP_BUDGET_S ))

SWAP_ENDPOINT="${SWAP_ENDPOINT:-http://127.0.0.1:8090}"

CONTAINER_NAME="${CONTAINER_NAME:-exam3-rocmfp4}"
CONTAINER_PORT="${CONTAINER_PORT:-18080}"
CONTAINER_ENDPOINT="http://127.0.0.1:$CONTAINER_PORT"
CONTAINER_ALIAS="rocmfp4-moe"
# Sampler parity with llama-swap's qwen36-moe entry (config.yaml). Without these
# the container ran llama.cpp defaults (top_k 40, min_p 0.05) while cell B ran
# top_k 20 / min_p 0.0, so the 2026-08-06 A-vs-B gap was partly a sampler gap.
# --temp here is a fallback only: the driver always sends `temperature`, which
# overrides it. top_p/top_k/min_p are never sent, so they must be set here.
IMAGE="${IMAGE:-local/rocmfpx-qwopus:gfx1150}"
MODEL_DIR="${MODEL_DIR:-/mnt/ai-models/llama/models/Qwen3.6-35B-A3B-ACE-SABER-ROCmFP4}"
MODEL_FILE="${MODEL_FILE:-Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf}"

# cell -> "display|endpoint|served-model-name"
cell_spec() {
  case "$1" in
    A) echo "rocmfp4-moe-35b-a3b|$CONTAINER_ENDPOINT|$CONTAINER_ALIAS" ;;
    B) echo "qwen36-moe-unsloth|$SWAP_ENDPOINT|qwen36-moe" ;;
    C) echo "gemma4-26b-qat|$SWAP_ENDPOINT|gemma4-26b-qat-mtp" ;;
    # --- hopper cells (R9700, 32 GiB dedicated VRAM, Vulkan) -----------------
    # Cells A-D above ran on the Framework 13 (62 GiB UMA). E-J are the same
    # exam on the R9700 box, same max_tokens/temp/seeds, reasoning ON.
    #
    # E is the continuity anchor and must run first: hopper's qwen-35b-moe-mtp
    # is the same build as cell B (Qwen3.6-35B-A3B UD-Q4_K_XL + MTP, 22.85 GB),
    # so it is the only cell whose score is directly comparable across machines.
    # Cell B scored 7/13 (seed42) and 5/13 (seed123). If E does not land near
    # that, the machines are not comparable and the rest of the table is not
    # worth running — stop and investigate rather than collecting numbers.
    E) echo "qwen36-moe-mtp-r9700|$SWAP_ENDPOINT|qwen-35b-moe-mtp" ;;
    F) echo "gemma4-31b-qat-r9700|$SWAP_ENDPOINT|gemma-31b-qat" ;;
    G) echo "gemma4-31b-ptq-r9700|$SWAP_ENDPOINT|gemma-31b" ;;
    H) echo "qwen36-27b-mtp-r9700|$SWAP_ENDPOINT|qwen-27b-mtp" ;;
    # Gemma 4 26B-A4B UD-Q5_K_XL, no MTP: hopper has no Gemma MTP/assistant
    # GGUF on disk and no QAT 26B build, so this is not the same weights as
    # cell C (QAT UD-Q4_K_XL + MTP). MTP is lossless for scoring — it moves
    # t/s and wall_s, not /13 — so the score is comparable to C, the timing
    # is not.
    I) echo "gemma4-26b-moe-r9700|$SWAP_ENDPOINT|gemma-26b-moe" ;;
    J) echo "gemma4-e4b-r9700|$SWAP_ENDPOINT|gemma-e4b" ;;
    # Qwen3.8-27B UD-Q4_K_XL, no MTP (none published for 3.8). Same quant and
    # same samplers as cell H, so K-vs-H is the generation delta 3.6 → 3.8 with
    # the quant held fixed. Timing is not comparable to H — H has a draft head.
    K) echo "qwen38-27b-r9700|$SWAP_ENDPOINT|qwen38-27b" ;;
    *) return 1 ;;
  esac
}

# Samplers sent explicitly by the driver on every cell (see exam-driver.go).
# Values match what cells A-C ran on the Framework 13, so cell E stays a valid
# continuity anchor. Note this overrides the *server* defaults for hopper's
# Gemma entries, which set no sampler flags and would otherwise run llama.cpp
# defaults (top_k 40, min_p 0.05) while the Qwen MTP entries run 20 / 0.0.
# Internal parity across the table is worth more than matching each model's
# own launch flags; the values land in result.json under `sampler` either way.
TOP_P="${TOP_P:-0.95}"
TOP_K="${TOP_K:-20}"
MIN_P="${MIN_P:-0.0}"

FIM_WAS_ACTIVE=0
mkdir -p "$LOG_ROOT" "$RESULTS_DIR"

log()  { printf '%s  %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$LOG_ROOT/sweep.log" >&2; }
fail() { log "ABORT: $*"; exit 1; }

out_of_time() { [ "$(date +%s)" -ge "$SWEEP_DEADLINE" ]; }

# --- environment control -----------------------------------------------------

# llama-swap's admin route differs by version: v211 (hopper) serves GET /unload
# and 404s the POST path; v241 (Framework 13) takes the POST. Try both.
swap_unload() {
  curl -fsS -m 30 "$SWAP_ENDPOINT/unload" >/dev/null 2>&1 && return 0
  curl -fsS -m 30 -X POST "$SWAP_ENDPOINT/api/models/unload" >/dev/null 2>&1
}

container_down() {
  docker stop --time 20 "$CONTAINER_NAME" >/dev/null 2>&1
  docker rm "$CONTAINER_NAME" >/dev/null 2>&1
  return 0
}

cleanup() {
  local rc=$?
  log "cleanup: tearing down"
  container_down
  swap_unload || true
  if [ "$FIM_WAS_ACTIVE" = "1" ]; then
    systemctl --user start llama-fim.service >/dev/null 2>&1
    log "cleanup: llama-fim restored -> $(systemctl --user is-active llama-fim.service)"
  fi
  log "cleanup: done (exit $rc)"
}
trap cleanup EXIT INT TERM

# --- preflight ---------------------------------------------------------------

# The evaluator must score the known-good reference solution 13/13. If it does
# not, every model score this sweep produces is meaningless. ~10s.
preflight_grader() {
  local probe response
  probe=$(mktemp -d)
  response="$probe/response.txt"
  python3 "$BENCH_DIR/make-reference-response.py" "$BENCH_DIR" "$response" \
    2>"$LOG_ROOT/preflight-reference.log"
  [ -s "$response" ] || { rm -rf "$probe"; fail "preflight: could not build reference response (see $LOG_ROOT/preflight-reference.log)"; }

  local out score
  out=$(bash "$BENCH_DIR/eval.sh" "$response" "$probe" 2>"$LOG_ROOT/preflight-grader.log")
  score=$(printf '%s' "$out" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("%d/%d"%(d["score"],d["max"]))' 2>/dev/null)
  rm -rf "$probe"
  [ "$score" = "13/13" ] || fail "preflight: evaluator scored reference solution $score, expected 13/13 (see $LOG_ROOT/preflight-grader.log)"
  log "preflight: evaluator scores reference solution 13/13"
}

preflight() {
  # docker is only needed for cell A (the ROCmFP4 container). The hopper cells
  # are all llama-swap, so don't gate them on a docker install.
  case " $CELLS " in
    *" A "*) command -v docker >/dev/null || fail "docker missing (needed for cell A)" ;;
  esac
  command -v go >/dev/null     || fail "go missing"
  command -v jq >/dev/null     || fail "jq missing (eval.sh needs it)"
  [ -f "$BENCH_DIR/prompt.txt" ] || fail "missing $BENCH_DIR/prompt.txt"
  [ -f "$BENCH_DIR/eval.sh" ]    || fail "missing $BENCH_DIR/eval.sh"
  [ -f "$DRIVER" ]               || fail "missing $DRIVER"

  # Every cell except A is served by llama-swap. Fail now rather than after the
  # ~10s grader preflight and a model load.
  if [ -n "$(printf '%s' "$CELLS" | tr -d ' A')" ]; then
    curl -fsS -m 10 "$SWAP_ENDPOINT/health" >/dev/null 2>&1 \
      || fail "llama-swap not responding at $SWAP_ENDPOINT"
  fi

  case " $CELLS " in
    *" A "*)
      [ -f "$MODEL_DIR/$MODEL_FILE" ] || fail "ROCmFP4 model missing: $MODEL_DIR/$MODEL_FILE"
      docker image inspect "$IMAGE" >/dev/null 2>&1 || fail "image missing: $IMAGE"
      ;;
  esac

  preflight_grader

  go build -o /dev/null "$DRIVER" || fail "exam-driver.go does not build"
  log "preflight: driver builds"

  if systemctl --user is-active --quiet llama-fim.service; then
    FIM_WAS_ACTIVE=1
    systemctl --user stop llama-fim.service
    log "preflight: llama-fim stopped (restored on exit)"
  fi

  container_down
  swap_unload || true
  sleep 3
  log "preflight: free mem $(free -g | awk '/^Mem:/{print $7}') GiB"
}

# --- cell A: ROCmFP4 container ----------------------------------------------

start_container() {
  log "cell A: starting $CONTAINER_NAME ($MODEL_FILE)"
  docker run -d --rm --name "$CONTAINER_NAME" \
    --device=/dev/kfd --device=/dev/dri \
    --group-add video --group-add render \
    --security-opt seccomp=unconfined \
    -v "$MODEL_DIR:/models:ro" \
    -p "127.0.0.1:$CONTAINER_PORT:$CONTAINER_PORT" \
    "$IMAGE" \
    -m "/models/$MODEL_FILE" \
    --alias "$CONTAINER_ALIAS" \
    --host 0.0.0.0 --port "$CONTAINER_PORT" \
    --flash-attn on --cache-type-k q8_0 --cache-type-v q8_0 \
    --gpu-layers 99 --no-mmap --ctx-checkpoints 0 --jinja --parallel 1 \
    --ctx-size 131072 --reasoning on --reasoning-budget 8192 \
    --predict 32768 --metrics --device ROCm0 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 --presence-penalty 0.0 \
    --spec-type draft-mtp --spec-draft-ngl all \
    --spec-draft-type-k q8_0 --spec-draft-type-v q8_0 --spec-draft-n-max 3 \
    >"$LOG_ROOT/container-id" 2>"$LOG_ROOT/container-start.err" \
    || fail "docker run failed: $(cat "$LOG_ROOT/container-start.err")"

  local i
  for i in $(seq 1 90); do
    if curl -fsS -m 5 "$CONTAINER_ENDPOINT/health" 2>/dev/null | grep -q '"ok"'; then
      log "cell A: healthy"
      docker logs "$CONTAINER_NAME" >"$LOG_ROOT/container-load.log" 2>&1
      return 0
    fi
    docker ps --format '{{.Names}}' | grep -q "^$CONTAINER_NAME$" \
      || { docker logs "$CONTAINER_NAME" >"$LOG_ROOT/container-load.log" 2>&1
           fail "container exited during load; see $LOG_ROOT/container-load.log"; }
    sleep 5
  done
  fail "container did not become healthy within 450s"
}

# --- attempts ----------------------------------------------------------------

# Cold prefill measured 1.07 t/s vs 13.10 warm. One throwaway request per cell
# absorbs the load+warm cost so wall_s reflects generation, not model loading.
warmup() {
  local endpoint="$1" model="$2"
  log "  warm-up (discarded)"
  curl -fsS -m 600 "$endpoint/v1/chat/completions" \
    -H 'content-type: application/json' \
    -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with OK.\"}],\"max_tokens\":8}" \
    >"$LOG_ROOT/warmup-$model.json" 2>&1 \
    || log "  WARN warm-up request failed (continuing)"
}

record_row() {
  local cell="$1" display="$2" served="$3" seed="$4" rj="$5" celllog="$6"
  python3 - "$cell" "$display" "$served" "$seed" "$rj" "$celllog" "$MAX_TOKENS" "$TEMP" >>"$STATUS_TSV" <<'PY'
import json, sys, datetime
cell, display, served, seed, path, celllog, maxtok, temp = sys.argv[1:9]
d = json.load(open(path))
e = d.get("eval", {})
summary = (e.get("summary") or "").replace("\n", " ").replace("\t", " ")
print("\t".join(str(x) for x in [
    datetime.datetime.now().isoformat(timespec="seconds"),
    cell, display, served, seed,
    e.get("score", ""), e.get("max", ""),
    d.get("tokens", ""),
    round(d.get("tps") or 0, 2),
    round(d.get("wall_s") or 0, 1),
    maxtok, temp,
    summary, celllog,
]))
PY
  tail -1 "$STATUS_TSV" | awk -F'\t' '{printf "  -> %s/%s  %s tok  %s t/s  %ss\n",$6,$7,$8,$9,$10}' >&2
}

run_attempt() {
  local cell="$1" display="$2" endpoint="$3" served="$4" seed="$5"
  local outdir="$RESULTS_DIR/$display/seed$seed"
  local celllog="$LOG_ROOT/${display}-seed${seed}.log"
  mkdir -p "$outdir"

  if [ -f "$outdir/result.json" ]; then
    log "  $display seed$seed: already present, skipping"
    record_row "$cell" "$display" "$served" "$seed" "$outdir/result.json" "$celllog"
    return 0
  fi

  log "  $display seed$seed: running (timeout $ATTEMPT_TIMEOUT)"
  local tmpout rc
  tmpout=$(mktemp -d)
  go run "$DRIVER" \
    -endpoint "$endpoint" \
    -prompt "$BENCH_DIR/prompt.txt" \
    -eval "$BENCH_DIR/eval.sh" \
    -out "$tmpout" \
    -seed "$seed" \
    -temp "$TEMP" \
    -top-p "$TOP_P" \
    -top-k "$TOP_K" \
    -min-p "$MIN_P" \
    -max-tokens "$MAX_TOKENS" \
    -timeout "$ATTEMPT_TIMEOUT" \
    "$served" >"$celllog" 2>&1
  rc=$?

  if [ -d "$tmpout/$served" ]; then
    cp -a "$tmpout/$served"/. "$outdir/" 2>/dev/null
  fi
  rm -rf "$tmpout"

  if [ ! -f "$outdir/result.json" ]; then
    log "  TRIPWIRE $display seed$seed: no result.json (driver rc=$rc); see $celllog"
    printf '%s\t%s\t%s\t%s\tNO_RESULT\tdriver_rc=%s\n' \
      "$(date -Is)" "$cell" "$display" "$seed" "$rc" >>"$LOG_ROOT/alerts.log"
    return 1
  fi
  record_row "$cell" "$display" "$served" "$seed" "$outdir/result.json" "$celllog"
}

# --- main --------------------------------------------------------------------

printf 'ts\tcell\tdisplay\tserved_model\tseed\tscore\tmax\ttokens\ttps\twall_s\tmax_tokens\ttemp\tsummary\tcell_log\n' >"$STATUS_TSV"

{
  echo "run_id:      $RUN_ID"
  echo "git_sha:     $(git -C "$LAB_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo "exam:        exam_v3 (one-shot, /13, unchanged prompt+evaluator)"
  echo "cells:       $CELLS"
  echo "seeds:       $SEEDS"
  echo "max_tokens:  $MAX_TOKENS"
  echo "temp:        $TEMP  (driver always sends it; April rerun also used 1.0)"
  echo "attempt_to:  $ATTEMPT_TIMEOUT"
  echo "budget_s:    $SWEEP_BUDGET_S"
  echo "reasoning:   ON for all cells (April rerun used --reasoning off)"
  echo "note:        max_tokens > reasoning-budget 8192, else content is empty"
  echo "sampler:     top_p=$TOP_P top_k=$TOP_K min_p=$MIN_P (sent by driver on every cell)"
  echo "host:        $(hostname)"
  echo "cellA:       $MODEL_DIR/$MODEL_FILE ($IMAGE, HIP gfx1150)"
  echo "cellB:       llama-swap qwen36-moe (Unsloth UD-Q4_K_XL, Vulkan)"
  echo "cellC:       llama-swap gemma4-26b-qat-mtp (QAT rebuild, Vulkan)"
} | tee "$LOG_ROOT/run-info.txt" >&2

preflight

for cell in $CELLS; do
  out_of_time && { log "sweep budget exhausted before cell $cell — stopping with partial results"; break; }

  spec=$(cell_spec "$cell") || fail "unknown cell '$cell' (expected A, B or C)"
  IFS='|' read -r display endpoint served <<<"$spec"

  log "===== cell $cell: $display (served as '$served' on $endpoint) ====="

  if [ "$cell" = "A" ]; then
    swap_unload || true
    start_container
  else
    container_down
  fi

  warmup "$endpoint" "$served"

  for seed in $SEEDS; do
    out_of_time && { log "budget exhausted mid-cell $cell"; break; }
    run_attempt "$cell" "$display" "$endpoint" "$served" "$seed" || true
  done

  if [ "$cell" = "A" ]; then
    container_down
    log "cell A: container down"
    sleep 5
  else
    swap_unload || true
    log "cell $cell: model unloaded"
  fi
done

log "sweep finished; status: $STATUS_TSV"
cut -f1-12 "$STATUS_TSV" | column -t -s$'\t' >&2
