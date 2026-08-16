#!/usr/bin/env bash
# exam_v4 on the `domain20` subset for five models, sequentially.
#
# Sequential is not a choice: llama-swap's group is swap=true exclusive=true,
# so only one model is resident at a time. Running these in parallel would
# thrash the GPU, not shorten the wall clock.
#
# Ordered by strat20 score, best first, so an interrupted sweep still yields
# the rows that matter: 17/46, 14/46, 14/46, 12/46, 10/46.
#
# Names are the post-2026-08-15 canonical ones, on two rules:
#   1. speculative decoding is on by default and is NOT in the name; the
#      exception is marked `-nodraft`.
#   2. Qwen entries carry their version, so `qwen36-*` and `qwen38-*` cannot be
#      confused. `qwen-27b` used to mean 3.6, which was a trap.
# `qwen36-27b` and `qwen36-35b-moe` here ARE the MTP entries — they appear as
# `qwen-27b-mtp` / `qwen-35b-moe-mtp` in the older strat20 table.
set -uo pipefail

LAB=$(cd "$(dirname "$0")/.." && pwd)
cd "$LAB"

MODELS=(qwen36-35b-moe qwen36-27b qwen38-27b gemma-31b-qat muse-glimmer-30b)
SUBSET=domain20   # built with --cap-agent-timeout 1200; see tb21-make-subset.py
LOGDIR="$LAB/artifacts/logs/domain20-sweep"
mkdir -p "$LOGDIR"

known=$(curl -fsS -m 10 http://127.0.0.1:8090/v1/models | python3 -c 'import json,sys; print(" ".join(m["id"] for m in json.load(sys.stdin)["data"]))') \
  || { echo "llama-swap not responding on :8090"; exit 1; }
for m in "${MODELS[@]}"; do
  case " $known " in *" $m "*) ;; *) echo "unknown model '$m' — known: $known"; exit 1;; esac
done

for m in "${MODELS[@]}"; do
  echo "==> $m start $(date -Is)"
  TERM=dumb NO_COLOR=1 ./scripts/tb21-run.sh "$m" "$SUBSET" \
    > "$LOGDIR/$m-$(date +%Y%m%d-%H%M%S).log" 2>&1 \
    || echo "!! $m exited $? — continuing"
  echo "==> $m done  $(date -Is)"
done

echo "==> sweep complete $(date -Is)"
