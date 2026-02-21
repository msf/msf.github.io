#!/usr/bin/env bash
set -euo pipefail

# Exam mode: all 3 programs in a single prompt, single timeout
LLAMA_CLI="${LLAMA_CLI:-llama-cli}"
MODEL_DIR="${MODEL_DIR:-$HOME/.cache/llama.cpp}"
CTX_SIZE="${CTX_SIZE:-8192}"
N_PREDICT="${N_PREDICT:--2}"
REASONING_BUDGET="${REASONING_BUDGET:--1}"
TIMEOUT="${TIMEOUT:-180}"
MODEL_FILTER="${MODEL_FILTER:-}"

OUTDIR="$(pwd)/bench_results"
mkdir -p "$OUTDIR"

# --- TEST INPUT ---
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

# Generate expected wordfreq
EXPECTED_WORDFREQ="$OUTDIR/expected_wordfreq.txt"
tr '[:upper:]' '[:lower:]' < "$TEST_INPUT" \
  | tr -cs '[:alpha:]' '\n' \
  | sort | uniq -c \
  | sort -rn \
  | head -10 \
  | awk '{printf "%s: %d\n", $2, $1}' \
  > "$EXPECTED_WORDFREQ"

# Expected filetree sizes
EXPECTED_FILETREE="$OUTDIR/expected_filetree_sizes.txt"
find "$HOME/.cache/llama.cpp" -type f ! -name '*mmproj*' -printf '%s\n' | sort -rn > "$EXPECTED_FILETREE"

# --- THE EXAM PROMPT ---
read -r -d '' EXAM_PROMPT <<'PROMPTEOF' || true
/no_think You are taking a timed coding exam. Write three complete, compilable Go programs. Each must be a standalone single-file program with package main and func main.

Output each file wrapped in markers exactly like this:
#START filename.go#
...go code...
#END filename.go#

The three programs:

1. factorial.go -- Computes 10! (ten factorial) and prints the result to stdout.

2. wordfreq.go -- Reads stdin line by line, counts word frequencies case-insensitively, prints the top 10 most frequent words sorted by count descending, one per line as "word: count".

3. filetreewalk.go -- Takes a directory path as its first command-line argument, recursively walks it, finds all regular files, sorts them by size descending, prints each as "SIZE PATH" one per line.

Be minimal, no comments, no explanation. Just the three files with their markers.
PROMPTEOF

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

echo "=== EXAM MODE ==="
echo "Models: ${#MODELS[@]}"
echo "Timeout: ${TIMEOUT}s (single prompt, 3 programs)"
echo ""

# --- EXTRACT FILES FROM MARKERS ---
extract_exam_files() {
  local raw_file="$1"
  local out_dir="$2"

  # Clean raw output
  local clean
  clean=$(cat "$raw_file" \
    | sed '/<think>/,/<\/think>/d' \
    | sed 's/<|[^>]*|>//g' \
    | sed 's/\[end of text\]//g')

  # Extract each file between #START name# and #END name#
  for fname in factorial.go wordfreq.go filetreewalk.go; do
    local content
    content=$(echo "$clean" | sed -n "/#START ${fname}#/,/#END ${fname}#/{/#START/d;/#END/d;p}") || true

    # Fallback: try without the trailing # (some models might format differently)
    if [ -z "$content" ]; then
      content=$(echo "$clean" | sed -n "/#START ${fname}/,/#END ${fname}/{/#START/d;/#END/d;p}") || true
    fi

    # Fallback: try with spaces around filename
    if [ -z "$content" ]; then
      content=$(echo "$clean" | sed -n "/#START *${fname} *#/,/#END *${fname} *#/{/#START/d;/#END/d;p}") || true
    fi

    # Strip any code fences that might be inside
    content=$(echo "$content" | sed '/^```/d')

    if [ -n "$content" ]; then
      echo "$content" > "$out_dir/$fname"
      echo "    Extracted: $fname ($(echo "$content" | wc -l) lines)"
    else
      echo "" > "$out_dir/$fname"
      echo "    MISSING: $fname"
    fi
  done
}

# --- RUN EACH MODEL ---
for model_path in "${MODELS[@]}"; do
  model_name=$(basename "$model_path" .gguf)
  short=$(echo "$model_name" | sed 's/.*GGUF_//')
  exam_dir="$OUTDIR/$short/exam"
  mkdir -p "$exam_dir"

  echo "============================================================"
  echo "EXAM: $short"
  echo "============================================================"

  raw_file="$exam_dir/exam.raw"
  stderr_file="$exam_dir/exam.stderr"
  time_file="$exam_dir/exam.time"

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
      --prompt "$EXAM_PROMPT" \
      > "$raw_file" 2> "$stderr_file" || true

  # Parse timings
  eval_tps=$(grep -oP '[\d.]+(?=\s*tokens per second)' "$stderr_file" | tail -1 || true)
  eval_tokens=$(grep -oP 'eval time\s*=.*?/\s*\K\d+(?=\s*tokens)' "$stderr_file" | tail -1 || true)
  wall_clock=$(grep 'Elapsed (wall clock)' "$time_file" | grep -oP '[\d:.]+$' || true)
  max_rss=$(grep 'Maximum resident set size' "$time_file" | grep -oP '\d+$' || true)
  max_rss_mb=""
  if [ -n "$max_rss" ]; then max_rss_mb=$(( max_rss / 1024 )); fi

  echo "  Tok/s: ${eval_tps:-?}  Tokens: ${eval_tokens:-?}  Wall: ${wall_clock:-?}  RSS: ${max_rss_mb:-?}MB"

  # Extract the 3 files
  extract_exam_files "$raw_file" "$exam_dir"

  # Build, run, verify each
  total_score=0
  for prog in factorial.go wordfreq.go filetreewalk.go; do
    gofile="$exam_dir/$prog"
    base="${prog%.go}"
    build_ok="FAIL"
    run_ok="FAIL"
    correct=""
    score=0

    if [ ! -s "$gofile" ]; then
      echo "  $prog: MISSING (0/5)"
      continue
    fi

    # Build (with auto-fix for missing closing brace)
    if go build -o "$exam_dir/${base}.bin" "$gofile" 2>"$exam_dir/${base}.build.log"; then
      build_ok="OK"
    elif grep -q 'expected }' "$exam_dir/${base}.build.log"; then
      echo "}" >> "$gofile"
      if go build -o "$exam_dir/${base}.bin" "$gofile" 2>"$exam_dir/${base}.build.log"; then
        build_ok="OK"
      fi
    fi

    if [ "$build_ok" != "OK" ]; then
      err=$(head -1 "$exam_dir/${base}.build.log")
      echo "  $prog: BUILD FAIL ($err) (0/5)"
      continue
    fi
    score=$((score + 1))

    # Run + verify
    case "$base" in
      factorial)
        if actual=$(timeout 5 "$exam_dir/${base}.bin" 2>&1); then
          run_ok="OK"; score=$((score + 1))
          trimmed=$(echo "$actual" | tr -d '[:space:]')
          if [ "$trimmed" = "3628800" ]; then
            correct="EXACT"; score=$((score + 2))
          else
            correct="WRONG($trimmed)"
          fi
        fi
        ;;
      wordfreq)
        if actual=$(timeout 5 "$exam_dir/${base}.bin" < "$TEST_INPUT" 2>&1); then
          run_ok="OK"; score=$((score + 1))
          top3_exp=$(head -3 "$EXPECTED_WORDFREQ" | grep -oP '^\S+' | sed 's/:$//' | sort)
          top3_act=$(echo "$actual" | head -3 | grep -oP '^\S+' | sed 's/:$//' | sort) || true
          if [ "$top3_exp" = "$top3_act" ]; then
            correct="EXACT"; score=$((score + 2))
          else
            correct="WRONG"
          fi
        fi
        ;;
      filetreewalk)
        if actual=$(timeout 10 "$exam_dir/${base}.bin" "$HOME/.cache/llama.cpp" 2>&1); then
          run_ok="OK"; score=$((score + 1))
          nlines=$(echo "$actual" | wc -l)
          nfiles_expected=$(wc -l < "$EXPECTED_FILETREE")
          sizes_ok="yes"
          prev=999999999999
          while read -r line; do
            sz=$(echo "$line" | grep -oP '^\d+') || true
            if [ -n "$sz" ] && [ "$sz" -gt "$prev" ]; then
              sizes_ok="no"; break
            fi
            [ -n "$sz" ] && prev="$sz"
          done <<< "$actual"
          if [ "$sizes_ok" = "yes" ] && [ "$nlines" -ge "$((nfiles_expected / 2))" ]; then
            correct="EXACT"; score=$((score + 2))
          else
            correct="WRONG"
          fi
        fi
        ;;
    esac

    # Error handling bonus
    if [ "$build_ok" = "OK" ]; then
      has_err=$(grep -c 'if err\|log.Fatal\|scanner.Err' "$gofile" || true)
      lines=$(wc -l < "$gofile")
      if [ "$has_err" -gt 0 ] && [ "$lines" -gt 5 ] && [ "$lines" -lt 80 ]; then
        score=$((score + 1))
      fi
    fi

    total_score=$((total_score + score))
    echo "  $prog: build=$build_ok run=$run_ok correct=${correct:-N/A} ($score/5)"
  done

  echo ""
  echo "  EXAM TOTAL: $total_score/15 (weighted: $((total_score * 5))/75)"

  # Save metadata
  cat > "$exam_dir/exam.json" <<METAEOF
{
  "model": "$short",
  "tps": "${eval_tps:-}",
  "tokens": "${eval_tokens:-}",
  "wall": "${wall_clock:-}",
  "rss_mb": "${max_rss_mb:-}",
  "exam_score": $total_score,
  "exam_weighted": $((total_score * 5))
}
METAEOF

  echo ""
done

# --- SUMMARY ---
echo "======================================================================"
echo "EXAM RESULTS"
echo "======================================================================"
printf "%-35s %7s %7s %8s %8s\n" "Model" "Tok/s" "Wall" "Score" "Weighted"
printf "%-35s %7s %7s %8s %8s\n" "-----" "-----" "----" "-----" "--------"
for model_path in "${MODELS[@]}"; do
  model_name=$(basename "$model_path" .gguf)
  short=$(echo "$model_name" | sed 's/.*GGUF_//')
  meta="$OUTDIR/$short/exam/exam.json"
  [ -f "$meta" ] || continue
  tps=$(jq -r '.tps // "?"' "$meta")
  wall=$(jq -r '.wall // "?"' "$meta")
  score=$(jq -r '.exam_score // "?"' "$meta")
  weighted=$(jq -r '.exam_weighted // "?"' "$meta")
  printf "%-35s %7s %7s %5s/15 %5s/75\n" "$short" "$tps" "$wall" "$score" "$weighted"
done
