#!/usr/bin/env bash
set -euo pipefail
#
# exam_v2 evaluator: extract code, compile, run Go integration test harness.
#
# Protocol:
#   eval.sh <response-file> <work-dir>
#   stdout: {"score":N, "max":M, "summary":"..."}
#   stderr: progress/debug
#
# Tests: TestScenario (5 subtests), TestMultipleOutageCycles,
#        TestBufferSizeZero, TestBufferSizeOne, TestGracefulShutdown, TestRaceDetector

RESPONSE_FILE="$(realpath "$1")"
WORK_DIR="$(realpath "$2")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Build mock server if needed ---
MOCK_BIN="$SCRIPT_DIR/mock/mockserver"
if [ ! -x "$MOCK_BIN" ] || [ "$SCRIPT_DIR/mock/main.go" -nt "$MOCK_BIN" ]; then
  echo "  building mock server..." >&2
  go build -o "$MOCK_BIN" "$SCRIPT_DIR/mock/main.go"
fi

# --- Extract code from response ---
extract_code() {
  local raw="$1" out="$2"
  local content

  # Strategy 1: last ```go block
  content=$(awk '
    /^```go/ { capture=1; block=""; next }
    /^```/ && capture { capture=0; last=block; next }
    capture { block = block (block ? "\n" : "") $0 }
    END { print last }
  ' "$raw")

  # Strategy 2: last package main to last }
  if [ -z "$content" ] || ! echo "$content" | grep -q 'package main'; then
    local pkg_line last_brace
    pkg_line=$(grep -n '^package main' "$raw" | tail -1 | cut -d: -f1) || true
    if [ -n "$pkg_line" ]; then
      content=$(tail -n +"$pkg_line" "$raw")
      last_brace=$(echo "$content" | grep -n '^}$' | tail -1 | cut -d: -f1) || true
      [ -n "$last_brace" ] && content=$(echo "$content" | head -n "$last_brace")
    fi
  fi

  if [ -n "$content" ] && echo "$content" | grep -q 'package main'; then
    # BUG 6 fix: models sometimes emit exam_v1-style #START/#END markers inside
    # the ```go fence. Strip them so they don't break Go compilation.
    content=$(echo "$content" | grep -vE '^#(START|END) ')
    echo "$content" > "$out"
    echo "  extracted $(echo "$content" | wc -l) lines" >&2
  else
    echo "" > "$out"
    echo "  EXTRACTION FAILED" >&2
  fi
}

# --- A. Extract and compile ---
gofile="$WORK_DIR/scraper.go"
extract_code "$RESPONSE_FILE" "$gofile"

if [ ! -s "$gofile" ]; then
  echo '{"score":0,"max":10,"summary":"extraction:FAIL"}'
  exit 0
fi

build_dir="$WORK_DIR/build"
mkdir -p "$build_dir"
cp "$gofile" "$build_dir/scraper.go"
echo 'module exam' > "$build_dir/go.mod"
echo 'go 1.23' >> "$build_dir/go.mod"

if ! (cd "$build_dir" && go build -o scraper . 2>"$WORK_DIR/build.log"); then
  echo "  compile: FAIL" >&2
  cat "$WORK_DIR/build.log" >&2
  echo '{"score":0,"max":10,"summary":"compile:FAIL"}'
  exit 0
fi
echo "  compile: OK" >&2

# --- B. Run integration test harness ---
HARNESS_DIR="$SCRIPT_DIR/harness"
SCRAPER_BIN="$(realpath "$build_dir/scraper")"

run_harness() {
  cd "$HARNESS_DIR" && go test -v -count=1 -timeout 600s . \
    -scraper-bin "$SCRAPER_BIN" \
    -mock-bin "$(realpath "$MOCK_BIN")" 2>&1
}

# Run up to twice if the first attempt produces no PASS/FAIL lines (harness
# flake, usually timing-sensitive test interactions).
test_output=$(run_harness) || true
if ! echo "$test_output" | grep -qE '\-\-\- (PASS|FAIL|SKIP):'; then
  echo "  retry: first harness run produced no results, retrying once" >&2
  sleep 2
  test_output=$(run_harness) || true
fi

# --- C. Parse test results ---
# Count PASS/FAIL for each test (including subtests)
pass=0
fail=0
skip=0
summary=""

# Count tests. Exclude the TestScenario parent (it's an aggregate over its
# subtests; counting both parent and subtests would double-count).
# Its subtests TestScenario/OnlineFlow etc. are what we actually score.
while IFS= read -r line; do
  case "$line" in
    *"--- PASS: TestScenario "*) continue ;;
    *"--- FAIL: TestScenario "*) continue ;;
    *"--- PASS:"*) pass=$((pass + 1)) ;;
    *"--- FAIL:"*) fail=$((fail + 1)) ;;
    *"--- SKIP:"*) skip=$((skip + 1)) ;;
  esac
done <<< "$test_output"

total=$((pass + fail + skip))
echo "  tests: $pass pass, $fail fail, $skip skip (of $total)" >&2

# BUG 7 fix: when go test returns zero PASS/FAIL/SKIP lines the harness broke
# (test binary crashed, wrong cwd, etc). Emit a distinct eval:FAIL status
# instead of a misleading 0/0.
if [ "$total" -eq 0 ]; then
  echo "  HARNESS FAILURE: no test results parsed" >&2
  echo "$test_output" | tail -20 >&2
  echo '{"score":0,"max":10,"summary":"eval:HARNESS_FAIL"}'
  exit 0
fi

# Build summary from test names (skip the TestScenario parent aggregate)
test_results=""
while IFS= read -r line; do
  if echo "$line" | grep -qE '\-\-\- (PASS|FAIL|SKIP):'; then
    name=$(echo "$line" | sed 's/.*--- [A-Z]*: //' | sed 's/ (.*//')
    # Skip the parent aggregate line
    if [ "$name" = "TestScenario" ]; then continue; fi
    status=$(echo "$line" | grep -oE '(PASS|FAIL|SKIP)' | head -1)
    test_results="$test_results $name:$status"
  fi
done <<< "$test_output"

echo "{\"score\":$pass,\"max\":$total,\"summary\":\"${test_results# }\"}"
