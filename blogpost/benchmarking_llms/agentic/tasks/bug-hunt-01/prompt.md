You are working in a small Go repository. It contains a metrics scraper that
reads from a solar inverter and forwards metrics to a remote sink, buffering
them in memory while the sink is unavailable.

There is exactly one behavioural bug. The test suite catches it.

Your task:

1. Explore the repository to understand it.
2. Run the tests to see the failure.
3. Fix the bug.
4. Run the tests again to confirm all of them pass.

Rules:

- Do not modify `grader_test.go`. It is the specification. Changes to it are
  discarded before your work is graded.
- Fix the implementation, not the test.
- `write_file` overwrites a whole file — always send complete file contents.
- When all tests pass, stop and reply with a one-paragraph explanation of the
  root cause. Do not keep calling tools after the suite is green.

You have these tools: `list_files`, `read_file`, `write_file`, `run_tests`.
There is no shell.
