#!/usr/bin/env bash
# exam_v4 — run a Terminal-Bench 2.1 subset against one llama-swap model.
#
#   tb21-run.sh <model> [subset]        e.g. tb21-run.sh gemma-31b-qat strat20
#
# One attempt per task, one trial at a time: llama-swap's exclusive group keeps
# a single model resident, so concurrency here would only thrash the GPU.
set -euo pipefail

MODEL=${1:?usage: tb21-run.sh <model> [subset]}
SUBSET=${2:-strat20}
LAB=$(cd "$(dirname "$0")/.." && pwd)

# The global `insteadOf` rule rewriting https->ssh breaks harbor's dataset clone.
export GIT_CONFIG_GLOBAL=/dev/null
# llama-swap needs no auth, but litellm's openai/ provider refuses to send a
# request unless a key is set to something.
export OPENAI_API_KEY=${OPENAI_API_KEY:-llama-swap-local}
export PATH="$HOME/.local/bin:$PATH"

exec harbor run \
  -a terminus-2 \
  -m "openai/$MODEL" \
  --ak api_base=http://127.0.0.1:8090/v1 \
  -n 1 \
  -q \
  -o "$LAB/artifacts/results/exam_v4_tb21/jobs" \
  -p "$LAB/artifacts/tb21-subsets/$SUBSET"
