#!/usr/bin/env bash
set -euo pipefail
export LC_NUMERIC=C
#
# exam_v1 evaluator: 3 simple Go programs (factorial, wordfreq, filetreewalk).
#
# Protocol:
#   eval.sh <response-file> <work-dir>
#   stdout: {"score":N, "max":15, "summary":"factorial:5/5 wordfreq:4/5 ..."}
#   stderr: progress/debug
#
# Per-program scoring (5 pts each, 15 total):
#   Build:      1 pt
#   Runs:       1 pt
#   Correct:    3 pts (graduated: partial credit for close answers)

RESPONSE="$1"
WORK_DIR="$2"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Extract files from #START/#END markers ---
extract_file() {
  local response="$1" fname="$2" outfile="$3"
  local content
  content=$(sed -n "/#START ${fname}#/,/#END ${fname}#/{/#START/d;/#END/d;p}" "$response") || true
  [ -z "$content" ] && content=$(sed -n "/#START ${fname}/,/#END ${fname}/{/#START/d;/#END/d;p}" "$response") || true
  content=$(echo "$content" | sed '/^```/d')
  if [ -n "$content" ]; then
    echo "$content" > "$outfile"
    echo "  extracted $fname ($(echo "$content" | wc -l) lines)" >&2
  else
    echo "" > "$outfile"
    echo "  MISSING: $fname" >&2
  fi
}

# --- Normalize text for word comparison ---
# Strips all non-alpha, lowercases, splits into words, sorts by frequency.
normalize_words() {
  tr '[:upper:]' '[:lower:]' | tr -cs '[:alpha:]' '\n' | grep -v '^$' | sort | uniq -c | sort -rn
}

# --- Prepare test data ---
TEST_INPUT="$SCRIPT_DIR/test_input.txt"

# Expected top words (normalized: strip punctuation, lowercase)
EXPECTED_WORDS=$(normalize_words < "$TEST_INPUT")
EXPECTED_TOP1=$(echo "$EXPECTED_WORDS" | head -1 | awk '{print $2}')
EXPECTED_TOP3=$(echo "$EXPECTED_WORDS" | head -3 | awk '{print $2}' | sort)
EXPECTED_TOP10=$(echo "$EXPECTED_WORDS" | head -10 | awk '{print $2}' | sort)

# Create temp tree for filetreewalk
TREE_DIR=$(mktemp -d)
trap "rm -rf '$TREE_DIR'" EXIT
mkdir -p "$TREE_DIR/sub"
dd if=/dev/zero of="$TREE_DIR/big.bin" bs=1024 count=100 2>/dev/null
dd if=/dev/zero of="$TREE_DIR/med.bin" bs=1024 count=50 2>/dev/null
dd if=/dev/zero of="$TREE_DIR/sub/small.bin" bs=1024 count=10 2>/dev/null
echo "hello" > "$TREE_DIR/sub/tiny.txt"
EXPECTED_FILE_COUNT=4

# --- Extract all 3 programs ---
extract_file "$RESPONSE" "factorial.go" "$WORK_DIR/factorial.go"
extract_file "$RESPONSE" "wordfreq.go" "$WORK_DIR/wordfreq.go"
extract_file "$RESPONSE" "filetreewalk.go" "$WORK_DIR/filetreewalk.go"

# --- Score each program ---
scores=()
details=()

for prog in factorial wordfreq filetreewalk; do
  gofile="$WORK_DIR/${prog}.go"
  binfile="$WORK_DIR/${prog}.bin"
  score=0

  if [ ! -s "$gofile" ]; then
    scores+=(0); details+=("${prog}:0/5(missing)")
    continue
  fi

  # Build (1 pt), with auto-fix for missing closing brace
  if go build -o "$binfile" "$gofile" 2>"$WORK_DIR/${prog}.build.log"; then
    score=$((score + 1))
  elif grep -q 'expected }' "$WORK_DIR/${prog}.build.log"; then
    echo "}" >> "$gofile"
    if go build -o "$binfile" "$gofile" 2>"$WORK_DIR/${prog}.build.log"; then
      score=$((score + 1))
    fi
  fi

  if [ "$score" -eq 0 ]; then
    scores+=(0); details+=("${prog}:0/5(build-fail)")
    echo "  $prog: build FAIL" >&2
    continue
  fi

  # Run + correctness (1 + 3 pts, graduated)
  case "$prog" in
    factorial)
      if actual=$(timeout 5 "$binfile" 2>&1); then
        score=$((score + 1))  # runs
        trimmed=$(echo "$actual" | tr -d '[:space:]')
        if [ "$trimmed" = "3628800" ]; then
          score=$((score + 3))  # exact match
        fi
      fi
      ;;

    wordfreq)
      if actual=$(timeout 5 "$binfile" < "$TEST_INPUT" 2>&1); then
        score=$((score + 1))  # runs

        # Normalize model output the same way as expected:
        # extract words (first non-whitespace token per line, strip trailing colon/punctuation)
        actual_words=$(echo "$actual" | head -10 | awk '{
          w = $1
          gsub(/[^a-zA-Z]/, "", w)
          if (w != "") print tolower(w)
        }' | head -10)
        actual_top1=$(echo "$actual_words" | head -1)
        actual_top3=$(echo "$actual_words" | head -3 | sort)
        actual_top10=$(echo "$actual_words" | head -10 | sort)

        if [ "$EXPECTED_TOP10" = "$actual_top10" ]; then
          score=$((score + 3))  # perfect top-10 match
        elif [ "$EXPECTED_TOP3" = "$actual_top3" ]; then
          score=$((score + 2))  # top-3 match
        elif [ "$EXPECTED_TOP1" = "$actual_top1" ]; then
          score=$((score + 1))  # at least top-1 correct
        fi
      fi
      ;;

    filetreewalk)
      if actual=$(timeout 10 "$binfile" "$TREE_DIR" 2>&1); then
        score=$((score + 1))  # runs
        nlines=$(echo "$actual" | wc -l)

        # Check descending sort by size
        sorted_ok="yes"
        prev=999999999999
        while read -r line; do
          sz=$(echo "$line" | grep -oP '^\d+') || true
          if [ -n "$sz" ] && [ "$sz" -gt "$prev" ]; then
            sorted_ok="no"; break
          fi
          [ -n "$sz" ] && prev="$sz"
        done <<< "$actual"

        # Check file count
        has_all_files="no"
        [ "$nlines" -ge "$EXPECTED_FILE_COUNT" ] && has_all_files="yes"

        if [ "$sorted_ok" = "yes" ] && [ "$has_all_files" = "yes" ]; then
          score=$((score + 3))  # sorted + all files
        elif [ "$sorted_ok" = "yes" ] && [ "$nlines" -ge 2 ]; then
          score=$((score + 2))  # sorted but missing some files
        elif [ "$nlines" -ge 1 ]; then
          score=$((score + 1))  # at least some output
        fi
      fi
      ;;
  esac

  scores+=("$score")
  details+=("${prog}:${score}/5")
  echo "  $prog: $score/5" >&2
done

total=0
for s in "${scores[@]}"; do total=$((total + s)); done
summary=$(IFS=' '; echo "${details[*]}")

echo "{\"score\":$total,\"max\":15,\"summary\":\"$summary\"}"
