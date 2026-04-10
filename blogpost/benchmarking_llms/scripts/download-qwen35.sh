#!/usr/bin/env bash
set -euo pipefail

# Warm llama.cpp's own Hugging Face cache for a small Qwen3.5 starter set.
# This uses llama.cpp directly so download/cache behavior matches the runtime
# you actually use.

LLAMA_CLI="${LLAMA_CLI:-$HOME/play/llama/llama-b8131/llama-cli}"
PRINT_ONLY="${PRINT_ONLY:-0}"
INCLUDE_SMALL_DENSE="${INCLUDE_SMALL_DENSE:-1}"
NGPU_LAYERS="${NGPU_LAYERS:-0}"

DENSE_REPO="${DENSE_REPO:-unsloth/Qwen3.5-9B-GGUF}"
DENSE_QUANTS="${DENSE_QUANTS:-Q4_K_M}"

MOE_REPO="${MOE_REPO:-unsloth/Qwen3.5-35B-A3B-GGUF}"
MOE_QUANTS="${MOE_QUANTS:-Q4_K_M}"

SMALL_REPO="${SMALL_REPO:-unsloth/Qwen3.5-4B-GGUF}"
SMALL_QUANTS="${SMALL_QUANTS:-Q4_K_M}"

split_csv() {
  tr ',' '\n' <<< "$1" | sed '/^$/d'
}

warm_repo() {
  local repo="$1"
  local quants="$2"
  local quant
  local cmd

  while IFS= read -r quant; do
    [[ -n "$quant" ]] || continue
    cmd=(
      "$LLAMA_CLI"
      --hf-repo "${repo}:${quant}"
      --no-mmproj
      --device none
      --gpu-layers "$NGPU_LAYERS"
      --predict 0
      --simple-io
      --prompt .
    )

    echo "==> ${repo}:${quant}"
    if [[ "$PRINT_ONLY" == "1" ]]; then
      printf '  %q' "${cmd[@]}"
      printf '\n\n'
    else
      "${cmd[@]}"
      echo
    fi
  done < <(split_csv "$quants")
}

if [[ ! -x "$LLAMA_CLI" ]]; then
  printf 'llama-cli not found or not executable: %s\n' "$LLAMA_CLI" >&2
  exit 1
fi

echo "Qwen3.5 cache warmer via llama.cpp"
echo "- llama-cli:    $LLAMA_CLI"
echo "- dense target: $DENSE_REPO ($DENSE_QUANTS)"
echo "- MoE target:   $MOE_REPO ($MOE_QUANTS)"
if [[ "$INCLUDE_SMALL_DENSE" == "1" ]]; then
  echo "- small dense:  $SMALL_REPO ($SMALL_QUANTS)"
fi
echo

warm_repo "$DENSE_REPO" "$DENSE_QUANTS"
warm_repo "$MOE_REPO" "$MOE_QUANTS"

if [[ "$INCLUDE_SMALL_DENSE" == "1" ]]; then
  warm_repo "$SMALL_REPO" "$SMALL_QUANTS"
fi

if [[ "$PRINT_ONLY" != "1" ]]; then
  echo "Done. Inspect cache with:"
  echo "  $LLAMA_CLI --cache-list"
  echo
  echo "Then benchmark with either:"
  echo "  MODEL_FILTER='Qwen3.5' ./bench.sh"
  echo "  MODEL_FILTER='Qwen3.5' ./exam.sh"
  echo
  echo "Quant sweep examples:"
  echo "  DENSE_QUANTS='Q4_K_M,Q6_K,Q8_0' INCLUDE_SMALL_DENSE=0 ./download-qwen35.sh"
  echo "  MOE_QUANTS='Q4_K_M,MXFP4_MOE' INCLUDE_SMALL_DENSE=0 ./download-qwen35.sh"
fi
