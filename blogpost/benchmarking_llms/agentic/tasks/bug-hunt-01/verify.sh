#!/usr/bin/env bash
set -uo pipefail
#
# verify.sh <sandbox-dir>
#
# Outcome scoring for bug-hunt-01. Exit 0 = passed, non-zero = failed.
#
# Anti-cheat: the agent can write any file, including grader_test.go. Before
# grading we restore the pristine grader and go.mod from the task definition,
# so a model that "fixes" the test instead of the code scores zero.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SANDBOX="${1:?usage: verify.sh <sandbox-dir>}"

if [ ! -d "$SANDBOX" ]; then
  echo "verify: sandbox not found: $SANDBOX"
  exit 2
fi

GRADE_DIR="$(mktemp -d)"
trap 'rm -rf "$GRADE_DIR"' EXIT

# Take the agent's tree, then overwrite the immutable files.
cp -r "$SANDBOX"/. "$GRADE_DIR"/
cp "$SCRIPT_DIR/repo/grader_test.go" "$GRADE_DIR/grader_test.go"
cp "$SCRIPT_DIR/repo/go.mod" "$GRADE_DIR/go.mod"

# Reject extra _test.go files: a model can otherwise neutralize the suite by
# redeclaring the stubs the grader depends on, or add a passing test elsewhere.
extra_tests="$(cd "$GRADE_DIR" && ls *_test.go 2>/dev/null | grep -v '^grader_test\.go$' || true)"
if [ -n "$extra_tests" ]; then
  echo "verify: FAIL — unexpected test files added: $extra_tests"
  exit 1
fi

out="$(cd "$GRADE_DIR" && GOFLAGS= GOWORK=off go test -count=1 -race -timeout 120s ./... 2>&1)"
rc=$?

echo "$out"

if [ "$rc" -ne 0 ]; then
  echo "verify: FAIL — go test exited $rc"
  exit 1
fi

if grep -q 'DATA RACE' <<<"$out"; then
  echo "verify: FAIL — data race detected"
  exit 1
fi

echo "verify: PASS"
exit 0
