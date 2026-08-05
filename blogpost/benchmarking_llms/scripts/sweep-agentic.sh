#!/usr/bin/env bash
set -uo pipefail
#
# Unattended agentic sweep: bug-hunt-01 across two Qwen3.6-35B-A3B servings.
#
#   cell A  rocmfp4-moe   ROCmFP4 ACE-SABER 19.0 GB, HIP, container on :18080
#   cell B  qwen36-moe    Unsloth UD-Q4_K_XL 22.9 GB, Vulkan, llama-swap on :8090
#
# Both cells: reasoning ON, --reasoning-budget 8192, MTP n-max 3, q8_0 KV,
# -c 131072. The driver sends NO sampler overrides, so each server's own
# --temp/--top-p/--top-k/--min-p governs. Parity was proven by matched prompt
# tokenization (10088 both sides) before this script existed.
#
# The two models cannot be resident at once (17.7 + 22.9 GiB on a 62 GiB box),
# so cells run sequentially: all of A, tear down, all of B.
#
# Usage:
#   ./sweep-agentic.sh                  # both cells, seeds 42 123 456
#   CELLS=A SEEDS=42 ./sweep-agentic.sh # one cell, one seed
#
# Idempotent: a cell/seed with an existing result.json is skipped, so an
# interrupted sweep resumes.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TASK="${TASK:-$LAB_ROOT/agentic/tasks/bug-hunt-01}"
DRIVER_DIR="$LAB_ROOT/agentic"
RESULTS_DIR="${RESULTS_DIR:-$LAB_ROOT/artifacts/results/agentic}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
LOG_ROOT="${LOG_ROOT:-$LAB_ROOT/artifacts/logs/agentic/$RUN_ID}"
STATUS_TSV="$LOG_ROOT/status.tsv"

SEEDS="${SEEDS:-42 123 456}"
CELLS="${CELLS:-A B}"

# Driver caps. per-call must exceed reasoning-budget(8192) + a whole-file
# write(~1.5k tokens): 4096 silently invalidated an earlier run.
PER_CALL="${PER_CALL:-16384}"
MAX_TURNS="${MAX_TURNS:-15}"
MAX_TOKENS="${MAX_TOKENS:-40000}"
ATTEMPT_TIMEOUT="${ATTEMPT_TIMEOUT:-25m}"

# Hard stop for the whole sweep. Partial results are kept.
SWEEP_BUDGET_S="${SWEEP_BUDGET_S:-14400}"   # 4h
SWEEP_DEADLINE=$(( $(date +%s) + SWEEP_BUDGET_S ))

SWAP_ENDPOINT="${SWAP_ENDPOINT:-http://127.0.0.1:8090}"
SWAP_MODEL="${SWAP_MODEL:-qwen36-moe}"

CONTAINER_NAME="${CONTAINER_NAME:-agentic-rocmfp4}"
CONTAINER_PORT="${CONTAINER_PORT:-18080}"
CONTAINER_ENDPOINT="http://127.0.0.1:$CONTAINER_PORT"
CONTAINER_ALIAS="rocmfp4-moe"
IMAGE="${IMAGE:-local/rocmfpx-qwopus:gfx1150}"
MODEL_DIR="${MODEL_DIR:-/mnt/ai-models/llama/models/Qwen3.6-35B-A3B-ACE-SABER-ROCmFP4}"
MODEL_FILE="${MODEL_FILE:-Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf}"

FIM_WAS_ACTIVE=0

mkdir -p "$LOG_ROOT"

log()  { printf '%s  %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$LOG_ROOT/sweep.log" >&2; }
fail() { log "ABORT: $*"; exit 1; }

out_of_time() {
  [ "$(date +%s)" -ge "$SWEEP_DEADLINE" ]
}

# --- environment control -----------------------------------------------------

swap_unload() {
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
  rm -rf /tmp/agentic-bug-hunt-01-* 2>/dev/null
  if [ "$FIM_WAS_ACTIVE" = "1" ]; then
    systemctl --user start llama-fim.service >/dev/null 2>&1
    log "cleanup: llama-fim restored -> $(systemctl --user is-active llama-fim.service)"
  fi
  log "cleanup: done (exit $rc)"
}
trap cleanup EXIT INT TERM

# --- preflight ---------------------------------------------------------------

preflight() {
  command -v docker >/dev/null || fail "docker missing"
  command -v go >/dev/null     || fail "go missing"
  [ -f "$TASK/prompt.md" ]     || fail "task prompt missing: $TASK/prompt.md"
  [ -x "$TASK/verify.sh" ]     || fail "verify.sh missing or not executable"
  [ -f "$MODEL_DIR/$MODEL_FILE" ] || fail "ROCmFP4 model missing: $MODEL_DIR/$MODEL_FILE"
  docker image inspect "$IMAGE" >/dev/null 2>&1 || fail "image missing: $IMAGE"

  # The fixture must still carry the injected defect, else every cell "passes".
  grep -q 'append((\*buf)\[1:\], m)' "$TASK/repo/resilient.go" \
    || fail "fixture defect absent from repo/resilient.go — task is not gradeable"
  if grep -q 'math/rand' "$TASK/repo/resilient.go"; then
    fail "fixture repo already imports math/rand — defect may have been reverted"
  fi

  # Broken fixture must fail, reference solution must pass. Cheap, ~5s, and it
  # is the only proof the grader still discriminates.
  local probe; probe=$(mktemp -d)
  cp "$TASK"/repo/* "$probe/"
  if bash "$TASK/verify.sh" "$probe" >"$LOG_ROOT/preflight-broken.log" 2>&1; then
    rm -rf "$probe"; fail "verify.sh PASSED the broken fixture — grader is not discriminating"
  fi
  cp "$TASK/solution/resilient.go" "$probe/resilient.go"
  if ! bash "$TASK/verify.sh" "$probe" >"$LOG_ROOT/preflight-fixed.log" 2>&1; then
    rm -rf "$probe"; fail "verify.sh FAILED the reference solution — grader is broken"
  fi
  rm -rf "$probe"
  log "preflight: grader discriminates (broken=FAIL, reference=PASS)"

  (cd "$DRIVER_DIR" && go build -o /dev/null ./agent-driver.go) \
    || fail "agent-driver.go does not build"
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
    --spec-type draft-mtp --spec-draft-ngl all \
    --spec-draft-type-k q8_0 --spec-draft-type-v q8_0 --spec-draft-n-max 3 \
    --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 --presence-penalty 0.0 \
    >"$LOG_ROOT/container-id" 2>"$LOG_ROOT/container-start.err" \
    || fail "docker run failed: $(cat "$LOG_ROOT/container-start.err")"

  local i
  for i in $(seq 1 90); do
    if curl -fsS -m 5 "$CONTAINER_ENDPOINT/health" 2>/dev/null | grep -q '"ok"'; then
      log "cell A: healthy after ${i}0s-ish"
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

# --- attempt execution -------------------------------------------------------

# Cold prefill measured 1.07 t/s vs 13.10 warm — a 12x artifact on wall_s.
# One throwaway request per cell absorbs it.
warmup() {
  local endpoint="$1" model="$2"
  log "  warm-up (discarded)"
  curl -fsS -m 300 "$endpoint/v1/chat/completions" \
    -H 'content-type: application/json' \
    -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with OK.\"}],\"max_tokens\":8}" \
    >"$LOG_ROOT/warmup-$model.json" 2>&1 \
    || log "  WARN warm-up request failed (continuing)"
}

run_attempt() {
  local cell="$1" endpoint="$2" model="$3" seed="$4"
  local outdir="$RESULTS_DIR/$(basename "$TASK")/$model/seed$seed"
  local celllog="$LOG_ROOT/${model}-seed${seed}.log"

  if [ -f "$outdir/result.json" ]; then
    log "  $model seed$seed: already present, skipping"
    record_row "$cell" "$model" "$seed" "$outdir/result.json" "$celllog"
    return 0
  fi

  log "  $model seed$seed: running (timeout $ATTEMPT_TIMEOUT)"
  local rc
  ( cd "$DRIVER_DIR" && go run ./agent-driver.go \
      -task "$TASK" \
      -endpoint "$endpoint" \
      -out "$RESULTS_DIR" \
      -seed "$seed" \
      -max-turns "$MAX_TURNS" \
      -max-tokens "$MAX_TOKENS" \
      -per-call-tokens "$PER_CALL" \
      -timeout "$ATTEMPT_TIMEOUT" \
      "$model" ) >"$celllog" 2>&1
  rc=$?

  if [ ! -f "$outdir/result.json" ]; then
    log "  TRIPWIRE $model seed$seed: no result.json (driver rc=$rc); see $celllog"
    printf '%s\t%s\t%s\t%s\tNO_RESULT\tdriver_rc=%s\n' \
      "$(date -Is)" "$cell" "$model" "$seed" "$rc" >>"$LOG_ROOT/alerts.log"
    return 1
  fi
  record_row "$cell" "$model" "$seed" "$outdir/result.json" "$celllog"
}

record_row() {
  local cell="$1" model="$2" seed="$3" rj="$4" celllog="$5"
  python3 - "$cell" "$model" "$seed" "$rj" "$celllog" >>"$STATUS_TSV" <<'PY'
import json, sys, datetime
cell, model, seed, path, celllog = sys.argv[1:6]
d = json.load(open(path))
cfg = d.get("harness_config", {})
print("\t".join(str(x) for x in [
    datetime.datetime.now().isoformat(timespec="seconds"),
    cell, model, seed,
    d.get("passed"),
    (d.get("stop_reason") or "")[:60].replace("\t", " ").replace("\n", " "),
    d.get("turns"), d.get("tool_calls"), d.get("tool_errors"),
    d.get("retry_count"), d.get("tool_parse_recoveries"),
    d.get("completion_tokens"), d.get("context_tokens_last"),
    d.get("reasoning_chars"),
    round(d.get("wall_s") or 0, 1),
    cfg.get("per_call_tokens"), cfg.get("max_turns"),
    celllog,
]))
PY
  tail -1 "$STATUS_TSV" | awk -F'\t' '{printf "  -> passed=%s stop=%s turns=%s errs=%s ctok=%s wall=%ss\n",$5,$6,$7,$9,$12,$15}' >&2
}

# --- main --------------------------------------------------------------------

printf 'ts\tcell\tmodel\tseed\tpassed\tstop_reason\tturns\ttool_calls\ttool_errors\tretries\trecoveries\tcompletion_tok\tctx_tok_last\treasoning_chars\twall_s\tper_call_tok\tmax_turns\tcell_log\n' >"$STATUS_TSV"

{
  echo "run_id:      $RUN_ID"
  echo "git_sha:     $(git -C "$LAB_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo "task:        $TASK"
  echo "seeds:       $SEEDS"
  echo "cells:       $CELLS"
  echo "per_call:    $PER_CALL"
  echo "max_turns:   $MAX_TURNS"
  echo "max_tokens:  $MAX_TOKENS"
  echo "attempt_to:  $ATTEMPT_TIMEOUT"
  echo "budget_s:    $SWEEP_BUDGET_S"
  echo "samplers:    omitted by driver; server-side config governs both cells"
  echo "cellA_model: $MODEL_DIR/$MODEL_FILE ($IMAGE, HIP gfx1150)"
  echo "cellB_model: llama-swap $SWAP_MODEL (Unsloth UD-Q4_K_XL, Vulkan)"
} | tee "$LOG_ROOT/run-info.txt" >&2

preflight

for cell in $CELLS; do
  out_of_time && { log "sweep budget exhausted before cell $cell — stopping with partial results"; break; }

  case "$cell" in
    A)
      swap_unload || true
      start_container
      warmup "$CONTAINER_ENDPOINT" "$CONTAINER_ALIAS"
      for seed in $SEEDS; do
        out_of_time && { log "budget exhausted mid-cell A"; break; }
        run_attempt A "$CONTAINER_ENDPOINT" "$CONTAINER_ALIAS" "$seed" || true
      done
      container_down
      log "cell A: container down"
      sleep 5
      ;;
    B)
      container_down
      warmup "$SWAP_ENDPOINT" "$SWAP_MODEL"
      for seed in $SEEDS; do
        out_of_time && { log "budget exhausted mid-cell B"; break; }
        run_attempt B "$SWAP_ENDPOINT" "$SWAP_MODEL" "$seed" || true
      done
      swap_unload || true
      log "cell B: model unloaded"
      ;;
    *) fail "unknown cell '$cell' (expected A or B)" ;;
  esac
done

log "sweep finished; status: $STATUS_TSV"
column -t -s$'\t' "$STATUS_TSV" | cut -c1-200 >&2
