#!/usr/bin/env python3
"""Rebuild a Terminal-Bench 2.1 task subset as a directory of symlinks.

The subset is the unit of comparison across models, so it must be reproducible:
harbor derives the suite name from the directory name, and the scoreboard merges
per-model results by that name. Rebuild rather than hand-curate.

  strat20  proportional stratified random sample, seed 42 (1 easy / 12 medium /
           7 hard). Proportional to the real 89-task mix and random *within*
           tier — an earlier hand-picked set biased toward short timeouts and
           over-weighted easy, and returned 83% where this returns 40%.

Usage: tb21-make-subset.py [--name strat20] [--size 20] [--seed 42]
"""

import argparse
import collections
import pathlib
import random
import re
import sys

LAB = pathlib.Path(__file__).resolve().parent.parent
TASKS = pathlib.Path.home() / ".cache/harbor/tasks/terminal-bench-2-1"
OUT = LAB / "artifacts/tb21-subsets"
TIMEOUT_RE = re.compile(r"(?:max_agent_timeout_sec|agent_timeout_sec|timeout_sec)\s*[:=]\s*([0-9.]+)")
DIFF_RE = re.compile(r'difficulty\s*[:=]\s*"?(\w+)"?')
TIERS = ("easy", "medium", "hard")


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


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name", default="strat20")
    ap.add_argument("--size", type=int, default=20)
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--tasks", type=pathlib.Path, default=TASKS)
    ap.add_argument("--out", type=pathlib.Path, default=OUT)
    a = ap.parse_args()

    if not a.tasks.is_dir():
        sys.exit(f"tasks not found: {a.tasks} — run: harbor download terminal-bench/terminal-bench-2-1 -o ~/.cache/harbor/tasks")

    pop = population(a.tasks)
    by = collections.defaultdict(list)
    for n, d, to in pop:
        by[d].append((n, to))

    random.seed(a.seed)
    pick = []
    for d in TIERS:
        k = max(1, round(a.size * len(by[d]) / len(pop)))
        pick += [(n, d, to) for n, to in random.sample(sorted(by[d]), min(k, len(by[d])))]

    dest = a.out / a.name
    if dest.exists():
        for p in dest.iterdir():
            p.unlink()
    dest.mkdir(parents=True, exist_ok=True)
    for n, _, _ in pick:
        (dest / n).symlink_to(a.tasks / n)

    c = collections.Counter(d for _, d, _ in pick)
    budget = sum(t[2] for t in pick)
    for n, d, to in sorted(pick, key=lambda x: (TIERS.index(x[1]), x[2])):
        print(f"   {d:7s} {to:6.0f}s  {n}")
    print(f"\n  {dest}")
    print(f"  n={len(pick)} {dict(c)}  worst-case budget {budget/3600:.1f}h")


if __name__ == "__main__":
    main()
