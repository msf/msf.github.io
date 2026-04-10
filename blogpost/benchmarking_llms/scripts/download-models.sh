#!/usr/bin/env bash
set -euo pipefail

CLI="${HOME}/play/llama/llama-current/llama-cli"

download() {
  local name="$1" repo="$2"
  echo "[$name] $repo"
  "$CLI" --hf-repo "$repo" \
    --gpu-layers 0 --n-predict 1 --single-turn --prompt "hi" --no-display-prompt \
    > /dev/null 2>"/tmp/dl-${name}.log"
  echo "[DONE] $name"
}

# Same repo = serialize. Different repos = parallel.
group_qwen35() {
  download qwen35-mxfp4 "unsloth/Qwen3.5-35B-A3B-GGUF:MXFP4_MOE"
  download qwen35-q5km  "unsloth/Qwen3.5-35B-A3B-GGUF:Q5_K_M"
  download qwen35-q6k   "unsloth/Qwen3.5-35B-A3B-GGUF:Q6_K"
}

group_qwen35 &
P1=$!
download gemma4-mxfp4 "unsloth/gemma-4-26B-A4B-it-GGUF:MXFP4_MOE" &
P2=$!

echo "Downloading... Monitor: tail -f /tmp/dl-*.log"
wait $P1 && wait $P2 && echo "All done." || echo "Some failed. Check /tmp/dl-*.log"
