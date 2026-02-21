#!/usr/bin/env bash
set -euo pipefail

# llm-code-bench: benchmark local LLMs on Go coding tasks
# Usage: ./bench.sh
# Override: LLAMA_CLI=/path/to/llama-cli MODEL_DIR=/path/to/models TIMEOUT=120 ./bench.sh
# Filter:   MODEL_FILTER="Qwen3" ./bench.sh  (run only matching models)

# --- CONFIG ---
LLAMA_CLI="${LLAMA_CLI:-llama-cli}"
MODEL_DIR="${MODEL_DIR:-$HOME/.cache/llama.cpp}"
CTX_SIZE="${CTX_SIZE:-4096}"
N_PREDICT="${N_PREDICT:--2}"
REASONING_BUDGET="${REASONING_BUDGET:--1}"
TIMEOUT="${TIMEOUT:-60}"
MODEL_FILTER="${MODEL_FILTER:-}"  # set to substring to run only matching models

OUTDIR="$(pwd)/bench_results"
mkdir -p "$OUTDIR"

# --- TEST INPUT FOR WORDFREQ ---
TEST_INPUT="$OUTDIR/test_input.txt"
if [ ! -f "$TEST_INPUT" ]; then
  cat > "$TEST_INPUT" <<'TXTEOF'
the quick brown fox jumps over the lazy dog
the dog barked at the cat and the cat hissed back
a quick red fox and a slow blue dog sat under the tree
the tree had many leaves and the leaves were green
every dog has its day and every cat has nine lives
the fox jumped over the fence and the dog chased the fox
under the old oak tree the cat slept while the dog watched
the quick brown dog and the lazy fox rested by the river
a dog and a cat and a fox walked into a bar
the bartender said we do not serve animals here
the cat replied but I am a paying customer said the cat
the dog barked twice and the fox said nothing at all
in the morning the dog chased the cat around the tree
the tree was tall and the dog was small but the dog jumped high
the fox watched from a distance and said the dog is brave
the cat climbed the tree and the dog waited below the tree
every morning the fox would run through the field
the field was full of flowers and the fox loved the flowers
the dog preferred the river and the cat preferred the tree
at the end of the day the dog the cat and the fox were friends
TXTEOF
fi

# Generate expected wordfreq output
EXPECTED_WORDFREQ="$OUTDIR/expected_wordfreq.txt"
tr '[:upper:]' '[:lower:]' < "$TEST_INPUT" \
  | tr -cs '[:alpha:]' '\n' \
  | sort | uniq -c \
  | sort -rn \
  | head -10 \
  | awk '{printf "%s: %d\n", $2, $1}' \
  > "$EXPECTED_WORDFREQ"

# Generate expected filetree reference (sizes descending)
EXPECTED_FILETREE="$OUTDIR/expected_filetree_sizes.txt"
find "$HOME/.cache/llama.cpp" -type f ! -name '*mmproj*' -printf '%s\n' | sort -rn > "$EXPECTED_FILETREE"

# --- 3 PROMPTS ---
TASK_NAMES=("factorial" "wordfreq" "filetree")

TASK_PROMPT_factorial='/no_think Output only Go code. A complete single-file Go program (package main) that calculates 10 factorial (10! = 10*9*8*7*6*5*4*3*2*1 = 3628800) and prints the result. Be minimal, no comments.'

TASK_PROMPT_wordfreq='/no_think Output only Go code. A complete single-file Go program (package main) that reads stdin line by line, counts word frequencies case-insensitively, prints the top 10 most frequent words sorted by count descending, one per line as "word: count". Be minimal, no comments.'

TASK_PROMPT_filetree='/no_think Output only Go code. A complete single-file Go program (package main) that takes a directory as its first command-line argument, recursively walks it, finds all regular files, sorts them by size descending, and prints each as "SIZE PATH" one per line. Be minimal, no comments.'

# --- FUNCTIONS ---

extract_go_code() {
  local raw_file="$1"
  local raw
  raw=$(cat "$raw_file")

  # Strip think blocks, model tokens, end markers
  local code
  code=$(echo "$raw" \
    | sed '/<think>/,/<\/think>/d' \
    | sed 's/<|[^>]*|>//g' \
    | sed 's/\[end of text\]//g' \
    | sed 's/package main/\npackage main/g' \
    | sed 's/^ *$//')

  # Try fenced Go block first (allow leading whitespace on fences)
  local fenced
  fenced=$(echo "$code" | sed -n '/^ *```go/,/^ *```/{/^ *```/d;p}' | head -100) || true
  if [ -z "$fenced" ]; then
    fenced=$(echo "$code" | sed -n '/^ *```/,/^ *```/{/^ *```/d;p}' | head -100) || true
  fi

  if [ -n "$fenced" ]; then
    echo "$fenced"
  else
    # Grab from last exact "package main" line (not "package main)" in prose)
    local last_pkg
    last_pkg=$(echo "$code" | grep -n '^package main$' | tail -1 | cut -d: -f1) || true
    if [ -n "$last_pkg" ]; then
      code=$(echo "$code" | tail -n +"$last_pkg")
    else
      # No package main found — prepend it if code starts with import/func
      local first_meaningful
      first_meaningful=$(echo "$code" | grep -m1 -nP '^\s*(import|func)' | cut -d: -f1) || true
      if [ -n "$first_meaningful" ]; then
        code=$(printf "package main\n\n%s" "$(echo "$code" | tail -n +"$first_meaningful")")
      fi
    fi
    echo "$code"
  fi
}

run_and_verify() {
  local task="$1"
  local bin="$2"
  local outfile="$3"
  local actual=""
  local run_ok="FAIL"
  local correct=""

  case "$task" in
    factorial)
      if actual=$(timeout 5 "$bin" 2>&1); then
        run_ok="OK"
        echo "$actual" > "$outfile"
        local trimmed
        trimmed=$(echo "$actual" | tr -d '[:space:]')
        if [ "$trimmed" = "3628800" ]; then
          correct="EXACT"
        else
          correct="WRONG"
        fi
      fi
      ;;
    wordfreq)
      if actual=$(timeout 5 "$bin" < "$TEST_INPUT" 2>&1); then
        run_ok="OK"
        echo "$actual" > "$outfile"
        # Compare top 3 words (unambiguous counts)
        local top3_expected top3_actual
        top3_expected=$(head -3 "$EXPECTED_WORDFREQ" | grep -oP '^\S+' | sed 's/:$//' | sort)
        top3_actual=$(echo "$actual" | head -3 | grep -oP '^\S+' | sed 's/:$//' | sort) || true
        if [ "$top3_expected" = "$top3_actual" ]; then
          # Check top 7 (all unambiguous)
          local top7_exp top7_act
          top7_exp=$(head -7 "$EXPECTED_WORDFREQ" | grep -oP '^\S+' | sed 's/:$//' | sort)
          top7_act=$(echo "$actual" | head -7 | grep -oP '^\S+' | sed 's/:$//' | sort) || true
          if [ "$top7_exp" = "$top7_act" ]; then
            correct="EXACT"
          else
            correct="PARTIAL"
          fi
        else
          correct="WRONG"
        fi
      fi
      ;;
    filetree)
      if actual=$(timeout 10 "$bin" "$HOME/.cache/llama.cpp" 2>&1); then
        run_ok="OK"
        echo "$actual" > "$outfile"
        # Check: output has lines, sizes are descending
        local nlines nfiles_expected
        nlines=$(echo "$actual" | wc -l)
        nfiles_expected=$(wc -l < "$EXPECTED_FILETREE")
        # Extract sizes from output (first number on each line)
        local sizes_ok="yes"
        local prev=999999999999
        while read -r line; do
          local sz
          sz=$(echo "$line" | grep -oP '^\d+') || true
          if [ -n "$sz" ]; then
            if [ "$sz" -gt "$prev" ]; then
              sizes_ok="no"
              break
            fi
            prev="$sz"
          fi
        done <<< "$actual"
        if [ "$sizes_ok" = "yes" ] && [ "$nlines" -ge "$((nfiles_expected / 2))" ]; then
          if [ "$nlines" -ge "$nfiles_expected" ]; then
            correct="EXACT"
          else
            correct="PARTIAL"
          fi
        else
          correct="WRONG"
        fi
      fi
      ;;
  esac

  echo "$run_ok|$correct"
}

parse_timings() {
  local stderr_file="$1"
  local time_file="$2"

  eval_tps=$(grep -oP '[\d.]+(?=\s*tokens per second)' "$stderr_file" | tail -1 || true)
  eval_tokens=$(grep -oP 'eval time\s*=.*?/\s*\K\d+(?=\s*tokens)' "$stderr_file" | tail -1 || true)
  wall_clock=$(grep 'Elapsed (wall clock)' "$time_file" | grep -oP '[\d:.]+$' || true)
  max_rss=$(grep 'Maximum resident set size' "$time_file" | grep -oP '\d+$' || true)
  if [ -n "$max_rss" ]; then
    max_rss_mb=$(( max_rss / 1024 ))
  else
    max_rss_mb=""
  fi
}

score_code() {
  local gofile="$1"
  local build_ok="$2"
  local run_ok="$3"
  local correct="$4"
  local score=0

  [ "$build_ok" = "OK" ] && score=$((score + 1))
  [ "$run_ok" = "OK" ] && score=$((score + 1))
  [ "$correct" = "EXACT" ] && score=$((score + 2))
  [ "$correct" = "PARTIAL" ] && score=$((score + 1))
  if [ "$build_ok" = "OK" ]; then
    local lines
    lines=$(wc -l < "$gofile")
    local has_err
    has_err=$(grep -c 'if err\|log.Fatal\|scanner.Err' "$gofile" || true)
    if [ "$lines" -gt 5 ] && [ "$lines" -lt 80 ] && [ "$has_err" -gt 0 ]; then
      score=$((score + 1))
    fi
  fi
  echo "$score"
}

# --- DISCOVER MODELS ---
MODELS=()
while IFS= read -r f; do
  base=$(basename "$f")
  case "$base" in
    *mmproj*) continue ;;
    *Coder-7B*) continue ;;
  esac
  if [ -n "$MODEL_FILTER" ]; then
    echo "$base" | grep -q "$MODEL_FILTER" || continue
  fi
  MODELS+=("$f")
done < <(find "$MODEL_DIR" -name '*.gguf' -type f -printf '%s %p\n' | sort -rn | awk '{print $2}')

if [ ${#MODELS[@]} -eq 0 ]; then
  echo "No models found" >&2
  exit 1
fi

echo "Models (${#MODELS[@]}):"
for m in "${MODELS[@]}"; do
  sz=$(stat -c%s "$m")
  echo "  $(basename "$m")  ($(( sz / 1024 / 1024 ))MB)"
done
echo ""
echo "Tasks: ${TASK_NAMES[*]}"
echo "Timeout: ${TIMEOUT}s per task"
echo ""

# --- MAIN LOOP: per model, per task ---
for model_path in "${MODELS[@]}"; do
  model_name=$(basename "$model_path" .gguf)
  short=$(echo "$model_name" | sed 's/.*GGUF_//')
  model_dir="$OUTDIR/$short"
  mkdir -p "$model_dir"

  echo "============================================================"
  echo "MODEL: $short"
  echo "============================================================"

  for task in "${TASK_NAMES[@]}"; do
    # Get prompt for this task
    prompt_var="TASK_PROMPT_${task}"
    prompt="${!prompt_var}"

    echo "  --- $task ---"

    raw_file="$model_dir/${task}.raw"
    stderr_file="$model_dir/${task}.stderr"
    time_file="$model_dir/${task}.time"
    gofile="$model_dir/${task}.go"
    output_file="$model_dir/${task}.output"

    # Run model
    /usr/bin/time -v -o "$time_file" \
      timeout "$TIMEOUT" \
      "$LLAMA_CLI" \
        --model "$model_path" \
        --ctx-size "$CTX_SIZE" \
        --predict "$N_PREDICT" \
        --reasoning-budget "$REASONING_BUDGET" \
        --temp 1 \
        --seed 42 \
        --no-display-prompt \
        --jinja \
        --single-turn \
        --prompt "$prompt" \
        > "$raw_file" 2> "$stderr_file" || true

    # Parse timings
    eval_tps="" eval_tokens="" wall_clock="" max_rss_mb=""
    parse_timings "$stderr_file" "$time_file"

    # Extract code
    extract_go_code "$raw_file" > "$gofile"

    # Build (auto-fix missing closing braces)
    build_ok="FAIL"
    if go build -o "$model_dir/${task}.bin" "$gofile" 2>"$model_dir/${task}.build.log"; then
      build_ok="OK"
    elif grep -q 'expected }' "$model_dir/${task}.build.log"; then
      echo "}" >> "$gofile"
      if go build -o "$model_dir/${task}.bin" "$gofile" 2>"$model_dir/${task}.build.log"; then
        build_ok="OK"
        echo "    (auto-fixed missing closing brace)"
      fi
    fi

    # Run + verify
    run_ok="FAIL"
    correct=""
    if [ "$build_ok" = "OK" ]; then
      result=$(run_and_verify "$task" "$model_dir/${task}.bin" "$output_file")
      run_ok=$(echo "$result" | cut -d'|' -f1)
      correct=$(echo "$result" | cut -d'|' -f2)
    fi

    # Score
    score=$(score_code "$gofile" "$build_ok" "$run_ok" "$correct")

    echo "    Tok/s: ${eval_tps:-?}  Tokens: ${eval_tokens:-?}  Wall: ${wall_clock:-?}  RSS: ${max_rss_mb:-?}MB"
    echo "    Build: $build_ok  Run: $run_ok  Correct: ${correct:-N/A}  Score: $score/5"

    # Save per-task metadata
    cat > "$model_dir/${task}.json" <<METAEOF
{
  "model": "$short",
  "task": "$task",
  "tps": "${eval_tps:-}",
  "tokens": "${eval_tokens:-}",
  "wall": "${wall_clock:-}",
  "rss_mb": "${max_rss_mb:-}",
  "build": "$build_ok",
  "run": "$run_ok",
  "correct": "${correct:-}",
  "score": $score
}
METAEOF

    # Show build errors briefly
    if [ "$build_ok" = "FAIL" ]; then
      head -3 "$model_dir/${task}.build.log" | sed 's/^/    > /'
    fi
  done
  echo ""
done

# --- FINAL SUMMARY ---
echo ""
echo "======================================================================"
echo "FINAL RESULTS"
echo "======================================================================"
echo ""

# Per-task tables
for task in "${TASK_NAMES[@]}"; do
  echo "--- $task ---"
  printf "%-35s %7s %6s %7s %5s %5s %7s %5s\n" "Model" "Tok/s" "Toks" "Wall" "Build" "Run" "Correct" "Score"
  printf "%-35s %7s %6s %7s %5s %5s %7s %5s\n" "-----" "-----" "----" "----" "-----" "---" "-------" "-----"
  for model_path in "${MODELS[@]}"; do
    model_name=$(basename "$model_path" .gguf)
    short=$(echo "$model_name" | sed 's/.*GGUF_//')
    meta="$OUTDIR/$short/${task}.json"
    [ -f "$meta" ] || continue
    tps=$(jq -r '.tps // "?"' "$meta")
    tokens=$(jq -r '.tokens // "?"' "$meta")
    wall=$(jq -r '.wall // "?"' "$meta")
    build=$(jq -r '.build // "?"' "$meta")
    run=$(jq -r '.run // "?"' "$meta")
    correct=$(jq -r '.correct // "?"' "$meta")
    score=$(jq -r '.score // "?"' "$meta")
    printf "%-35s %7s %6s %7s %5s %5s %7s %3s/5\n" "$short" "$tps" "$tokens" "$wall" "$build" "$run" "$correct" "$score"
  done
  echo ""
done

# Overall ranking
echo "--- OVERALL RANKING ---"
printf "%-35s %7s %5s  %s\n" "Model" "AvgTk/s" "Total" "Tasks"
printf "%-35s %7s %5s  %s\n" "-----" "-------" "-----" "-----"
for model_path in "${MODELS[@]}"; do
  model_name=$(basename "$model_path" .gguf)
  short=$(echo "$model_name" | sed 's/.*GGUF_//')
  total_score=0
  total_tps=0
  tps_count=0
  task_results=""
  for task in "${TASK_NAMES[@]}"; do
    meta="$OUTDIR/$short/${task}.json"
    if [ -f "$meta" ]; then
      s=$(jq -r '.score' "$meta")
      total_score=$((total_score + s))
      t=$(jq -r '.tps // ""' "$meta")
      if [ -n "$t" ] && [ "$t" != "null" ]; then
        total_tps=$(echo "$total_tps + $t" | bc)
        tps_count=$((tps_count + 1))
      fi
      task_results="$task_results ${task}:${s}/5"
    fi
  done
  if [ "$tps_count" -gt 0 ]; then
    avg_tps=$(echo "scale=1; $total_tps / $tps_count" | bc)
  else
    avg_tps="?"
  fi
  printf "%-35s %7s %3s/15 %s\n" "$short" "$avg_tps" "$total_score" "$task_results"
done | sort -t'/' -k1 -rn -k1,1
