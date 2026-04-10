#!/usr/bin/env bash
set -euo pipefail
#
# exam_v2 evaluator: extract code, compile in-process with grader_test.go,
# run `go test -race`, score PASS/FAIL leaf tests.
#
# Protocol:
#   eval.sh <response-file> <work-dir>
#   stdout: {"score":N, "max":M, "summary":"..."}
#   stderr: progress/debug
#
# Scored leaf tests (max=13):
#   TestNewScraperValidation
#   TestReadsDuringOutage
#   TestGracefulCancel
#   TestNoLossAcrossTransitions
#   TestShortOutageNoLoss
#   TestMultipleShortOutagesNoLoss
#   TestLongOutage/BoundedBuffer
#   TestLongOutage/FullBufferFlushed
#   TestLongOutage/EvictionNotContiguous
#   TestSurvivesUnderLoad
#   TestHangBehavior/CancelDuringHungRead
#   TestHangBehavior/CancelDuringHungWrite
#   TestHangBehavior/ReadsProgressDespiteHungWrite
# Parent aggregate lines (TestLongOutage, TestHangBehavior) are excluded.

RESPONSE_FILE="$(realpath "$1")"
WORK_DIR="$(realpath "$2")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GRADER="$SCRIPT_DIR/grader_test.go"

if [ ! -f "$GRADER" ]; then
  echo "missing grader: $GRADER" >&2
  echo '{"score":0,"max":13,"summary":"eval:NO_GRADER"}'
  exit 0
fi

MAX=13

# --- Extract Go code from response ---
# Strategy 1: last ```go ... ``` fenced block (the output format mandated by prompt.txt).
# Strategy 2: fallback — from last `package main` to last standalone `}`.
extract_code() {
  local raw="$1" out="$2"
  local content

  content=$(awk '
    /^```go[[:space:]]*$/ { capture=1; block=""; next }
    /^```[[:space:]]*$/ && capture { capture=0; last=block; next }
    capture { block = block (block ? "\n" : "") $0 }
    END { print last }
  ' "$raw")

  if [ -z "$content" ] || ! echo "$content" | grep -q '^package main'; then
    local pkg_line last_brace
    pkg_line=$(grep -n '^package main' "$raw" | tail -1 | cut -d: -f1) || true
    if [ -n "$pkg_line" ]; then
      content=$(tail -n +"$pkg_line" "$raw")
      last_brace=$(echo "$content" | grep -n '^}$' | tail -1 | cut -d: -f1) || true
      [ -n "$last_brace" ] && content=$(echo "$content" | head -n "$last_brace")
    fi
  fi

  if [ -n "$content" ] && echo "$content" | grep -q '^package main'; then
    # Some models still emit exam_v1-style #START/#END markers; strip them.
    content=$(echo "$content" | grep -vE '^#(START|END) ')
    echo "$content" > "$out"
    echo "  extracted $(echo "$content" | wc -l) lines" >&2
  else
    echo "" > "$out"
    echo "  EXTRACTION FAILED" >&2
  fi
}

# --- Extract + stage ---
gofile="$WORK_DIR/scraper.go"
extract_code "$RESPONSE_FILE" "$gofile"

if [ ! -s "$gofile" ]; then
  echo "{\"score\":0,\"max\":$MAX,\"summary\":\"extraction:FAIL\"}"
  exit 0
fi

build_dir="$WORK_DIR/build"
mkdir -p "$build_dir"
cp "$gofile" "$build_dir/scraper.go"
cp "$GRADER" "$build_dir/grader_test.go"
printf 'module exam\ngo 1.23\n' > "$build_dir/go.mod"

# --- Run the grader ---
# `go test -race -json` compiles scraper.go + grader_test.go together as
# package main and emits one JSON event per line. We parse events with jq.
test_log="$WORK_DIR/test.json"
test_raw="$WORK_DIR/test.log" # human-readable tail for debugging
if ! (cd "$build_dir" && go test -count=1 -race -timeout 60s -json . > "$test_log" 2>&1); then
  # Nonzero exit: compile failure, test failure, or race. JSON parsing below
  # distinguishes these from harness breakage.
  :
fi
# Keep a human-readable tail for debug if something goes wrong.
jq -r 'select(.Output) | .Output' "$test_log" > "$test_raw" 2>/dev/null || true

# --- Detect compile failure ---
# `build-fail` action appears when the test binary won't link.
if jq -e 'select(.Action == "build-fail")' "$test_log" > /dev/null 2>&1; then
  echo "  compile: FAIL" >&2
  jq -r 'select(.Action == "build-output") | .Output' "$test_log" | head -20 >&2
  echo "{\"score\":0,\"max\":$MAX,\"summary\":\"compile:FAIL\"}"
  exit 0
fi

# --- Count test results ---
# Leaf tests only: exclude the two parent aggregates (TestLongOutage,
# TestHangBehavior) that wrap subtests.
parents='TestLongOutage|TestHangBehavior'
results_line=$(jq -rc --arg parents "$parents" '
  select(.Test != null)
  | select(.Action == "pass" or .Action == "fail" or .Action == "skip")
  | select(.Test | test("^(" + $parents + ")$") | not)
  | "\(.Test):\(.Action | ascii_upcase)"
' "$test_log" | tr '\n' ' ' | sed 's/ $//')

pass=$(jq -rc --arg parents "$parents" '
  select(.Test != null and .Action == "pass")
  | select(.Test | test("^(" + $parents + ")$") | not)
  | .Test
' "$test_log" | wc -l)

fail=$(jq -rc --arg parents "$parents" '
  select(.Test != null and .Action == "fail")
  | select(.Test | test("^(" + $parents + ")$") | not)
  | .Test
' "$test_log" | wc -l)

observed=$((pass + fail))
echo "  tests: $pass pass, $fail fail (of $observed observed; max=$MAX)" >&2

# --- Race detection ---
race_note=""
if grep -q 'DATA RACE' "$test_raw"; then
  race_count=$(grep -c '==================' "$test_raw" || true)
  echo "  DATA RACE detected ($race_count markers)" >&2
  race_note=" race:DETECTED"
fi

if [ "$observed" -eq 0 ]; then
  echo "  HARNESS FAILURE: no test results parsed" >&2
  tail -20 "$test_raw" >&2
  echo "{\"score\":0,\"max\":$MAX,\"summary\":\"eval:HARNESS_FAIL\"}"
  exit 0
fi

# Fixed denominator = MAX. Unrun tests (early panic) count as failures.
echo "{\"score\":$pass,\"max\":$MAX,\"summary\":\"${results_line}${race_note}\",\"observed\":$observed}"
