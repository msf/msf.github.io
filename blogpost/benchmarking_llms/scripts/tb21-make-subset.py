#!/usr/bin/env python3
"""Rebuild a Terminal-Bench 2.1 task subset as a directory of symlinks.

The subset is the unit of comparison across models, so it must be reproducible:
harbor derives the suite name from the directory name, and the scoreboard merges
per-model results by that name. Rebuild rather than hand-curate.

  strat20  proportional stratified random sample, seed 42 (1 easy / 12 medium /
           7 hard). Proportional to the real 89-task mix and random *within*
           tier — an earlier hand-picked set biased toward short timeouts and
           over-weighted easy, and returned 83% where this returns 40%.

  domain20 same sampling, but drawn from the 54-task in-domain pool rather than
           all 89 (--exclude-offdomain). Scoped 2026-08-15 to the work this box
           is actually used for: Linux/Go/Rust systems, self-hosted infra,
           security. NOT a fairer benchmark — a narrower one. Scores are not
           comparable to strat20; it is a different population.

Agent timeouts in terminal-bench are not set to any consistent policy: across
the 89 tasks they range 900-12000 s, and the ratio of budget to the task's own
expert-time estimate spans 0.06x to 3.3x. --cap-agent-timeout imposes one.
20 min is the working choice: past that an agent is looping, out of its depth,
or on a task that does not belong in a 21-task examination. terminus-2 also has
no context management, so long runs degrade rather than progress.

Usage: tb21-make-subset.py [--name strat20] [--size 20] [--seed 42]
                           [--exclude-offdomain] [--cap-agent-timeout 1200]
"""

import argparse
import collections
import pathlib
import random
import shutil
import re
import sys

LAB = pathlib.Path(__file__).resolve().parent.parent
TASKS = pathlib.Path.home() / ".cache/harbor/tasks/terminal-bench-2-1"
OUT = LAB / "artifacts/tb21-subsets"
TIMEOUT_RE = re.compile(r"(?:max_agent_timeout_sec|agent_timeout_sec|timeout_sec)\s*[:=]\s*([0-9.]+)")
DIFF_RE = re.compile(r'difficulty\s*[:=]\s*"?(\w+)"?')
TIERS = ("easy", "medium", "hard")

# Tasks excluded by --exclude-offdomain. Scoped by Miguel 2026-08-15 against the
# work this box is used for. Kept as an explicit name list, not a category
# filter: terminal-bench's own `category` field cuts across these (both FEAL
# cryptanalysis tasks are category "mathematics", `circuit-fibsqrt` is
# "software-engineering"), so filtering by it gives the wrong set.
#
# Deliberately NOT excluded, though they look borderline:
#   compile-compcert, build-pov-ray, build-cython-ext, sqlite-with-gcov —
#     subject matter is incidental; the work is toolchains and build failures.
#   make-mips-interpreter, make-doom-for-mips — low-level systems programming.
#   llm-inference-batching-scheduler, torch-{pipeline,tensor}-parallelism,
#     count-dataset-tokens, hf-model-inference — this box runs a local LLM
#     stack; inference infra is on-domain.
#   code-from-image, financial-document-processor, extract-moves-from-video,
#     video-processing — vision/OCR kept by explicit request.
OFFDOMAIN = {
    # exotic / legacy languages and toolchains
    "cobol-modernization", "fix-ocaml-gc", "prove-plus-comm",
    "schemelike-metacircular-eval", "winning-avg-corewars", "build-pmars",
    "gcode-to-text", "overfull-hbox", "install-windows-3.11",
    # cryptanalysis and pure maths
    "feal-differential-cryptanalysis", "feal-linear-cryptanalysis",
    "model-extraction-relu-logits", "largest-eigenval", "circuit-fibsqrt",
    # esoteric puzzle-code. The two polyglot-* tasks are "one source file that
    # compiles as both X and Y and computes Fibonacci" — same species.
    # polyglot-rust-c is additionally the only task in all 89 tagged
    # `no-verified-solution`, i.e. not known to be solvable at all.
    "regex-chess", "gpt2-codegolf", "chess-best-move",
    "polyglot-rust-c", "polyglot-c-py",
    # wet-lab / domain science
    "dna-assembly", "dna-insert", "protein-assembly", "raman-fitting",
    "tune-mjcf", "sam-cell-seg",
    # Bayesian / R statistics
    "bn-fit-modify", "mcmc-sampling-stan", "rstan-to-pystan",
    "adaptive-rejection-sampler", "distribution-search",
    # ML research and training
    "caffe-cifar-10", "train-fasttext", "mteb-leaderboard", "mteb-retrieve",
    "pytorch-model-recovery",
    # rendering math
    "path-tracing", "path-tracing-reverse",
    # --- dropped 2026-08-15 after auditing pass criteria, not subject matter ---
    # sparql-university: expert estimate 800 min against a 900 s agent budget
    #   (53x), pass is exact set-equality against hardcoded REFERENCE_RESULTS,
    #   and it requires memorised EU membership as of 2025-08-16 — world
    #   knowledge, not engineering. Not winnable.
    # count-dataset-tokens: asserts `"79586" in answer.txt` — a substring check
    #   that also accepts "179586" — while needing live HuggingFace dataset and
    #   tokenizer downloads mid-task and one exact integer. Brittle and
    #   false-positive-prone at the same time.
    # break-filter-js-from-html: pass requires headless Chrome + Selenium to
    #   observe an alert() firing; adversarial XSS bypass that hinges on
    #   recalling one trick. Observed unstable: passed in the un-drafted
    #   qwen38 run and timed out in the drafted one, same model, same night.
    "sparql-university", "count-dataset-tokens", "break-filter-js-from-html",
    # build-pov-ray was dropped here on 2026-08-15 for a 12000 s agent timeout
    # (13x the subset median; measured at 200.5 min for a single failed task).
    # Re-admitted the same day once --cap-agent-timeout existed: the problem was
    # the budget, not the task, and the cap fixes it for every task uniformly.
}


def population(tasks_dir):
    out = []
    for t in sorted(tasks_dir.iterdir()):
        if not t.is_dir():
            continue
        f = t / "task.toml"
        if not f.exists():
            f = t / "task.yaml"
        if not f.exists():
            continue
        txt = f.read_text()
        to = TIMEOUT_RE.search(txt)
        d = DIFF_RE.search(txt)
        out.append((t.name, d.group(1) if d else "?", float(to.group(1)) if to else 0.0))
    return out


def cap_agent_timeout(text, cap):
    """Lower `timeout_sec` under [agent] to `cap`. Other sections untouched.

    Section-aware on purpose: task.toml carries `timeout_sec` under both
    [verifier] and [agent], plus `build_timeout_sec`. Only the agent's thinking
    budget is capped — the verifier still needs its full allowance to build and
    run the tests, and starving it would turn slow test suites into fake
    failures.
    """
    out, section = [], None
    for line in text.splitlines(keepends=True):
        s = line.strip()
        if s.startswith("[") and s.endswith("]"):
            section = s[1:-1]
        elif section == "agent":
            m = re.match(r'(\s*timeout_sec\s*=\s*)([0-9.]+)(.*)$', line)
            if m and float(m.group(2)) > cap:
                line = f"{m.group(1)}{cap}{m.group(3)}\n"
        out.append(line)
    return "".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name", default="strat20")
    ap.add_argument("--size", type=int, default=20)
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--tasks", type=pathlib.Path, default=TASKS)
    ap.add_argument("--out", type=pathlib.Path, default=OUT)
    ap.add_argument("--exclude-offdomain", action="store_true",
                    help="draw only from the in-domain pool (see OFFDOMAIN)")
    ap.add_argument("--extend", metavar="NAME",
                    help="build a superset of an existing subset: keep all of its "
                         "tasks and draw only the remainder up to --size")
    ap.add_argument("--extend-tier", metavar="TIERS",
                    help="with --extend, draw the new tasks only from these tiers "
                         "(comma-separated, e.g. medium)")
    ap.add_argument("--delta-only", action="store_true",
                    help="with --extend, emit ONLY the newly drawn tasks. Lets a "
                         "model already scored on the base subset be topped up "
                         "without re-running tasks it has already done; score the "
                         "two together with tb21-scoreboard.py --suite a,b")
    ap.add_argument("--cap-agent-timeout", type=float, default=None, metavar="SEC",
                    help="cap each task's [agent] timeout_sec at SEC. Materialises "
                         "the subset as copies instead of symlinks, since the "
                         "harbor task cache is shared and must not be edited.")
    a = ap.parse_args()

    if not a.tasks.is_dir():
        sys.exit(f"tasks not found: {a.tasks} — run: harbor download terminal-bench/terminal-bench-2-1 -o ~/.cache/harbor/tasks")

    pop = population(a.tasks)
    if a.exclude_offdomain:
        unknown = OFFDOMAIN - {n for n, _, _ in pop}
        if unknown:
            sys.exit(f"OFFDOMAIN names not in task pool: {sorted(unknown)}")
        pop = [t for t in pop if t[0] not in OFFDOMAIN]
        print(f"  in-domain pool: {len(pop)} of {len(pop) + len(OFFDOMAIN)} tasks")

    random.seed(a.seed)

    if a.extend:
        # Superset mode: keep every task of an existing subset and draw only the
        # remainder. This is what makes an extension affordable — runs already
        # scored on the base subset stay valid, so a model only needs the new
        # tasks, not all of them.
        src = a.out / a.extend
        if not src.is_dir():
            sys.exit(f"--extend: no such subset: {src}")
        base = sorted(p.name for p in src.iterdir() if p.is_dir() or p.is_symlink())
        index = {n: (n, d, to) for n, d, to in pop}
        missing = [n for n in base if n not in index]
        if missing:
            sys.exit(f"--extend: base tasks absent from the current pool "
                     f"(off-domain filter changed?): {missing}")
        need = a.size - len(base)
        if need <= 0:
            sys.exit(f"--extend: --size {a.size} is not larger than {a.extend} ({len(base)})")

        rest = [t for t in pop if t[0] not in set(base)]
        if a.extend_tier:
            want = set(a.extend_tier.split(","))
            bad = want - set(TIERS)
            if bad:
                sys.exit(f"--extend-tier: unknown tier(s): {sorted(bad)}")
            rest = [t for t in rest if t[1] in want]
        if need > len(rest):
            sys.exit(f"--extend: need {need} more tasks, only {len(rest)} available"
                     + (f" in tier(s) {a.extend_tier}" if a.extend_tier else ""))

        new = random.sample(sorted(rest), need)
        pick = new if a.delta_only else [index[n] for n in base] + new
        print(f"  extending {a.extend}: {len(base)} kept + {need} new"
              + (f" ({a.extend_tier})" if a.extend_tier else "")
              + ("  [delta-only: emitting the new tasks alone]" if a.delta_only else ""))
    else:
        by = collections.defaultdict(list)
        for n, d, to in pop:
            by[d].append((n, to))
        pick = []
        for d in TIERS:
            k = max(1, round(a.size * len(by[d]) / len(pop)))
            pick += [(n, d, to) for n, to in random.sample(sorted(by[d]), min(k, len(by[d])))]

    dest = a.out / a.name
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True, exist_ok=True)
    for n, _, _ in pick:
        if a.cap_agent_timeout is None:
            (dest / n).symlink_to(a.tasks / n)
        else:
            shutil.copytree(a.tasks / n, dest / n, symlinks=True)
            f = dest / n / "task.toml"
            f.write_text(cap_agent_timeout(f.read_text(), a.cap_agent_timeout))

    # Report the timeouts actually written, not the upstream ones — a summary
    # that still shows 12000s after capping is worse than no summary.
    cap = a.cap_agent_timeout
    eff = [(n, d, min(to, cap) if cap else to) for n, d, to in pick]
    c = collections.Counter(d for _, d, _ in eff)
    budget = sum(t[2] for t in eff)
    for n, d, to in sorted(eff, key=lambda x: (TIERS.index(x[1]), x[2])):
        capped = " (capped)" if cap and to < next(o for m, _, o in pick if m == n) else ""
        print(f"   {d:7s} {to:6.0f}s  {n}{capped}")
    print(f"\n  {dest}")
    print(f"  n={len(eff)} {dict(c)}  worst-case agent budget {budget/3600:.1f}h"
          + (f"  (agent timeout capped at {cap:.0f}s)" if cap else ""))


if __name__ == "__main__":
    main()
