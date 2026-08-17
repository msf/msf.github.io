#!/usr/bin/env python3
"""Build the exam_v4 (Terminal-Bench 2.1) scoreboard from harbor job directories.

Scans artifacts/results/exam_v4_tb21/jobs/*/ and emits a markdown table to stdout
plus a self-contained HTML page. Regenerate after every run — the table is
derived, never hand-edited, so it cannot drift from the artifacts.

Scoring reads verifier/reward.txt, NOT the exception type: a trial can raise
AgentTimeoutError and still score 1 when partial work satisfies the tests
(observed on model-extraction-relu-logits, 2026-08-07).

Usage: tb21-scoreboard.py [--html OUT.html] [--jobs DIR] [--tasks DIR]
"""

import argparse
import collections
import datetime
import json
import pathlib
import re
import sys

LAB = pathlib.Path(__file__).resolve().parent.parent
JOBS = LAB / "artifacts/results/exam_v4_tb21/jobs"
TASKS = pathlib.Path.home() / ".cache/harbor/tasks/terminal-bench-2-1"
TIMEOUT_RE = re.compile(r"(?:max_agent_timeout_sec|agent_timeout_sec|timeout_sec)\s*[:=]\s*([0-9.]+)")
DIFF_RE = re.compile(r'difficulty\s*[:=]\s*"?(\w+)"?')
TIERS = ("easy", "medium", "hard")
# Weighted score: a hard task is worth three easy ones. Flat pass-rate treats
# them alike, which flatters a model that only clears the easy tier.
POINTS = {"easy": 1, "medium": 2, "hard": 3}


def task_meta(tasks_dir):
    """slug -> (difficulty, timeout_seconds) for every task on disk."""
    meta = {}
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
        meta[t.name] = (d.group(1) if d else "?", float(to.group(1)) if to else 0.0)
    return meta


def read_jobs(jobs_dir, meta):
    """One record per (model, task) trial, newest job wins on duplicates."""
    runs = []
    for job in sorted(jobs_dir.iterdir()):
        cfg = job / "config.json"
        if not cfg.exists():
            continue
        c = json.loads(cfg.read_text())
        agents = c.get("agents") or [{}]
        model = agents[0].get("model_name", "?").replace("openai/", "")
        agent = agents[0].get("name", "?")
        dsets = c.get("datasets") or [{}]
        suite = pathlib.Path(dsets[0].get("path", "?")).name
        trials = {}
        for d in sorted(job.glob("*__*")):
            slug = d.name.rsplit("__", 1)[0]
            rw = d / "verifier/reward.txt"
            reward = None
            if rw.exists():
                try:
                    reward = float(rw.read_text().strip())
                except ValueError:
                    reward = None
            exc = ""
            rj = d / "result.json"
            if rj.exists():
                try:
                    e = json.loads(rj.read_text()).get("exception_info") or {}
                    exc = (e.get("exception_type") if isinstance(e, dict) else str(e)) or ""
                except json.JSONDecodeError:
                    pass
            trials[slug] = {"reward": reward, "exception": exc,
                            "difficulty": meta.get(slug, ("?", 0))[0],
                            "timeout": meta.get(slug, ("?", 0))[1]}
        # Wall time for the whole job, plus tokens. A job still running has no
        # finished_at, so fall back to updated_at and mark it incomplete.
        wall, done, tok_in, tok_out = None, True, None, None
        rj = job / "result.json"
        if rj.exists():
            try:
                j = json.loads(rj.read_text())
                start = j.get("started_at")
                end = j.get("finished_at") or j.get("updated_at")
                done = bool(j.get("finished_at"))
                if start:
                    # harbor mixes naive (job level) and Z-suffixed (trial level)
                    # timestamps; compare them as naive. For a job still running,
                    # `updated_at` can lag `started_at` and yield a negative
                    # duration, so measure elapsed against now instead.
                    a = datetime.datetime.fromisoformat(start).replace(tzinfo=None)
                    if done and end:
                        b = datetime.datetime.fromisoformat(end).replace(tzinfo=None)
                    else:
                        b = datetime.datetime.now()
                    wall = (b - a).total_seconds()
                    if wall < 0:
                        wall = None
                st = j.get("stats") or {}
                tok_in, tok_out = st.get("n_input_tokens"), st.get("n_output_tokens")
            except (json.JSONDecodeError, ValueError):
                pass
        if trials:
            runs.append({"job": job.name, "model": model, "agent": agent,
                         "suite": suite, "trials": trials, "wall": wall,
                         "done": done, "tok_in": tok_in, "tok_out": tok_out})
    return runs


def fmt_dur(s):
    if not s:
        return "—"
    h, m = divmod(int(s) // 60, 60)
    return f"{h}h{m:02d}m" if h else f"{m}m"


def merge_runs(runs, suite):
    """Collapse a model's jobs into one record, newest job winning per task.

    A rerun of a single task (e.g. after GPU contention invalidated it) is its
    own job, so per-model results must merge across jobs rather than the later
    job replacing the earlier one wholesale. Wall time sums across jobs.

    `suite` may be a set of names, which unions them into one table. That is how
    a superset subset is scored without re-running its shared tasks: `domain30`
    is the `domain20` jobs plus the `domain30-delta` jobs. Only sound when the
    shared task definitions are byte-identical between the two, which was
    checked by hashing every shared task.toml before the delta was run.
    """
    suites = suite if isinstance(suite, (set, frozenset, list, tuple)) else {suite}
    merged = {}
    for r in runs:  # runs arrive in job-name order, so later jobs overwrite
        if r["suite"] not in suites:
            continue
        m = merged.setdefault(r["model"], {"trials": {}, "wall": 0.0, "done": True,
                                           "jobs": [], "tok_in": 0, "tok_out": 0})
        m["trials"].update(r["trials"])
        m["wall"] += r["wall"] or 0
        m["done"] = m["done"] and r["done"]
        m["jobs"].append(r["job"])
        m["tok_in"] += r["tok_in"] or 0
        m["tok_out"] += r["tok_out"] or 0
    return merged


def summarize(runs, suite):
    """model -> per-tier and overall pass counts, for one suite (or set of them)."""
    out = {}
    for model, r in merge_runs(runs, suite).items():
        r = dict(r, model=model)
        tier = collections.defaultdict(lambda: [0, 0])
        for slug, t in r["trials"].items():
            ok = t["reward"] == 1.0
            tier[t["difficulty"]][0] += int(ok)
            tier[t["difficulty"]][1] += 1
        p = sum(v[0] for v in tier.values())
        n = sum(v[1] for v in tier.values())
        pts = sum(POINTS.get(t, 0) * v[0] for t, v in tier.items())
        pts_max = sum(POINTS.get(t, 0) * v[1] for t, v in tier.items())
        out[r["model"]] = {"tier": dict(tier), "pass": p, "n": n, "job": r["jobs"][-1],
                           "points": pts, "points_max": pts_max,
                           "wall": r["wall"], "done": r["done"],
                           "tok_in": r["tok_in"], "tok_out": r["tok_out"]}
    return out


def by_points(kv):
    """Rank on weighted points, not flat pass rate."""
    s = kv[1]
    return -(s["points"] / s["points_max"] if s["points_max"] else 0)


def md_table(summary, suite):
    w = " · ".join(f"{t}={POINTS[t]}" for t in TIERS)
    lines = [f"### {suite}  (points: {w})", "",
             "| model | " + " | ".join(TIERS) + " | score | tasks | runtime | per task |",
             "|---|" + "---|" * (len(TIERS) + 4)]
    tot_wall = 0
    for m, s in sorted(summary.items(), key=by_points):
        cells = []
        for t in TIERS:
            p, n = s["tier"].get(t, [0, 0])
            cells.append(f"{p}/{n}" if n else "—")
        ppct = s["points"] / s["points_max"] * 100 if s["points_max"] else 0
        per = fmt_dur(s["wall"] / s["n"]) if s["wall"] and s["n"] else "—"
        run = fmt_dur(s["wall"]) + ("" if s["done"] else " (running)")
        tot_wall += s["wall"] or 0
        lines.append(f"| `{m}` | " + " | ".join(cells)
                     + f" | **{s['points']}/{s['points_max']}** ({ppct:.0f}%)"
                     + f" | {s['pass']}/{s['n']} | {run} | {per} |")
    lines.append("")
    lines.append(f"total GPU time across all models: **{fmt_dur(tot_wall)}**")
    return "\n".join(lines)


HTML_HEAD = """<title>exam_v4 — Terminal-Bench 2.1 on the R9700</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
/* Warm-slate neutrals with a copper accent: the box is an AMD workstation, and
   the subject is a terminal benchmark, so mono carries the display voice.
   Pass/fail stay semantic and separate from the accent. */
:root{
  --paper:#faf8f6; --ink:#1b1815; --mut:#6d655d; --line:#e6e0d9; --card:#fff;
  --accent:#a9552a; --pass:#2c7a52; --fail:#b03f31; --chip:#f0ebe5;
}
@media (prefers-color-scheme:dark){:root{
  --paper:#15130f; --ink:#ece7e1; --mut:#9b928a; --line:#2e2a25; --card:#1d1a16;
  --accent:#e08a54; --pass:#5fbc8b; --fail:#e8796a; --chip:#26221d;
}}
:root[data-theme=dark]{
  --paper:#15130f; --ink:#ece7e1; --mut:#9b928a; --line:#2e2a25; --card:#1d1a16;
  --accent:#e08a54; --pass:#5fbc8b; --fail:#e8796a; --chip:#26221d;
}
:root[data-theme=light]{
  --paper:#faf8f6; --ink:#1b1815; --mut:#6d655d; --line:#e6e0d9; --card:#fff;
  --accent:#a9552a; --pass:#2c7a52; --fail:#b03f31; --chip:#f0ebe5;
}
*{box-sizing:border-box}
body{background:var(--paper);color:var(--ink);
  font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
  margin:0 auto;padding:1.25rem 1rem 3rem;max-width:54rem;
  -webkit-text-size-adjust:100%}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
h1{font:600 1.35rem/1.2 ui-monospace,SFMono-Regular,Menlo,monospace;margin:0 0 .3rem;
   letter-spacing:-.02em;text-wrap:balance}
h2{font-size:.78rem;text-transform:uppercase;letter-spacing:.09em;color:var(--mut);
   margin:2.25rem 0 .75rem;font-weight:600}
.sub{color:var(--mut);font-size:.82rem;line-height:1.5;margin-bottom:.5rem}
.sub b{color:var(--ink);font-weight:600}

/* summary: cards, so a phone never scrolls sideways to read the headline number */
.cards{display:grid;gap:.7rem;grid-template-columns:1fr}
@media(min-width:34rem){.cards{grid-template-columns:1fr 1fr}}
.card{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:.85rem .95rem}
.card .name{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.85rem;
  font-weight:600;word-break:break-all;margin-bottom:.5rem}
.big{display:flex;align-items:baseline;gap:.5rem;margin-bottom:.6rem}
.pct{font:600 2rem/1 ui-monospace,SFMono-Regular,Menlo,monospace;
  font-variant-numeric:tabular-nums;letter-spacing:-.03em}
.of{color:var(--mut);font-size:.85rem;font-variant-numeric:tabular-nums}
.track{height:.4rem;background:var(--chip);border-radius:99px;overflow:hidden;margin-bottom:.7rem}
.fill{height:100%;background:var(--accent);border-radius:99px}
.tiers{display:flex;gap:.4rem;flex-wrap:wrap}
.tier{background:var(--chip);border-radius:5px;padding:.2rem .45rem;font-size:.73rem;
  color:var(--mut);font-variant-numeric:tabular-nums}
.tier b{color:var(--ink);font-weight:600}
.tier i{font-style:normal;color:var(--accent);font-weight:600;margin-left:.25rem;font-size:.68rem}
.wnote{float:none;display:inline;margin-left:.5rem;text-transform:none;letter-spacing:0;
  font-weight:400;color:var(--mut);font-size:.72rem}
.runtime{display:flex;justify-content:space-between;gap:.5rem;margin-top:.6rem;
  padding-top:.6rem;border-top:1px solid var(--line);font-size:.73rem;color:var(--mut);
  font-variant-numeric:tabular-nums}
.runtime b{color:var(--ink);font-weight:600;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.live{font-size:.62rem;text-transform:uppercase;letter-spacing:.06em;font-weight:600;
  color:var(--accent);border:1px solid color-mix(in srgb,var(--accent) 40%,transparent);
  border-radius:4px;padding:.1rem .3rem;margin-left:.4rem;font-family:inherit;white-space:nowrap}

/* per-task: one row per task, models as chips — reads top-to-bottom on a phone */
.task{border-bottom:1px solid var(--line);padding:.6rem 0}
.task:last-child{border-bottom:0}
.thead{display:flex;align-items:baseline;gap:.5rem;flex-wrap:wrap;margin-bottom:.35rem}
.slug{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.82rem;
  word-break:break-all;flex:1 1 auto;min-width:0}
.meta{font-size:.7rem;color:var(--mut);font-variant-numeric:tabular-nums;white-space:nowrap}
.tag{font-size:.65rem;text-transform:uppercase;letter-spacing:.05em;padding:.1rem .35rem;
  border-radius:4px;background:var(--chip);color:var(--mut);font-weight:600}
.chips{display:flex;gap:.35rem;flex-wrap:wrap}
.chip{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.68rem;
  padding:.18rem .42rem;border-radius:5px;border:1px solid transparent;white-space:nowrap}
.chip.ok{color:var(--pass);border-color:color-mix(in srgb,var(--pass) 35%,transparent);
  background:color-mix(in srgb,var(--pass) 10%,transparent)}
.chip.no{color:var(--fail);border-color:color-mix(in srgb,var(--fail) 30%,transparent);
  background:color-mix(in srgb,var(--fail) 8%,transparent)}
.chip.na{color:var(--mut);border-color:var(--line)}
.chip .t{opacity:.75;font-size:.62rem}
.note{color:var(--mut);font-size:.78rem;line-height:1.55;border-left:2px solid var(--accent);
  padding-left:.8rem;margin:1.5rem 0 0}
.note code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.72rem}
</style>
"""


def _short(model):
    """Chip label: drop the shared prefixes that carry no distinguishing signal."""
    return model.replace("qwen-", "q").replace("gemma-", "g").replace("-moe", "").replace("-mtp", "*")


def html_page(summary, label, runs, meta, suites):
    now = datetime.datetime.now().strftime("%Y-%m-%d %H:%M")
    rows = sorted(summary.items(), key=by_points)
    weights = " · ".join(f"{t} {POINTS[t]}pt" for t in TIERS)
    out = [HTML_HEAD,
           "<h1>exam_v4 · Terminal-Bench 2.1</h1>",
           f'<div class="sub">hopper · Radeon AI PRO R9700 · agent <span class="mono">terminus-2</span> · '
           f'reasoning <b>on</b> · {len(rows)} model(s) · updated {now}</div>']

    out.append(f'<h2>Weighted score <span class="wnote">{weights}</span></h2><div class="cards">')
    total_wall = 0
    for m, s in rows:
        ppct = s["points"] / s["points_max"] * 100 if s["points_max"] else 0
        total_wall += s["wall"] or 0
        tiers = ""
        for t in TIERS:
            p, n = s["tier"].get(t, [0, 0])
            if n:
                tiers += (f'<span class="tier">{t} <b>{p}/{n}</b>'
                          f'<i>{p * POINTS[t]}pt</i></span>')
        per = fmt_dur(s["wall"] / s["n"]) if s["wall"] and s["n"] else "—"
        live = "" if s["done"] else '<span class="live">running</span>'
        out.append(
            f'<div class="card"><div class="name">{m}{live}</div>'
            f'<div class="big"><span class="pct">{s["points"]}</span>'
            f'<span class="of">of {s["points_max"]} pts · {ppct:.0f}%</span></div>'
            f'<div class="track"><div class="fill" style="width:{ppct:.0f}%"></div></div>'
            f'<div class="tiers">{tiers}</div>'
            f'<div class="runtime"><span>{s["pass"]}/{s["n"]} tasks · <b>{fmt_dur(s["wall"])}</b></span>'
            f'<span>{per}/task</span></div></div>')
    out.append("</div>")
    out.append(f'<div class="sub" style="margin-top:.8rem">Total GPU time so far: '
               f'<b>{fmt_dur(total_wall)}</b> across {len(rows)} model(s), '
               f'{sum(s["n"] for _, s in rows)} trials.</div>')

    models = [m for m, _ in rows]
    slugs = sorted({s for v in merge_runs(runs, suites).values() for s in v["trials"]},
                   key=lambda s: (TIERS.index(meta.get(s, ("hard", 0))[0])
                                  if meta.get(s, ("?", 0))[0] in TIERS else 9,
                                  meta.get(s, ("?", 0))[1]))
    bym = {m: v["trials"] for m, v in merge_runs(runs, suites).items()}

    out.append("<h2>Per task</h2><div>")
    for slug in slugs:
        d, to = meta.get(slug, ("?", 0))
        chips = ""
        for m in models:
            t = bym.get(m, {}).get(slug)
            if not t:
                chips += f'<span class="chip na">{_short(m)} —</span>'
            else:
                ok = t["reward"] == 1.0
                # A timeout that still scored 1 is a real and non-obvious case; mark it.
                flag = ' <span class="t">t/o</span>' if t["exception"] else ""
                chips += (f'<span class="chip {"ok" if ok else "no"}">'
                          f'{_short(m)} {"pass" if ok else "fail"}{flag}</span>')
        out.append(f'<div class="task"><div class="thead">'
                   f'<span class="slug">{slug}</span>'
                   f'<span class="tag">{d}</span>'
                   f'<span class="meta">{to:.0f}s</span></div>'
                   f'<div class="chips">{chips}</div></div>')
    out.append("</div>")

    out.append('<div class="note">'
               'Scored from <code>verifier/reward.txt</code>, not the exception type — a trial can raise '
               '<code>AgentTimeoutError</code> and still score 1 when partial work satisfies the tests. '
               'Chips marked <span class="mono">t/o</span> hit their agent timeout regardless of outcome.'
               '<br><br>'
               'Score weights each solved task by tier: easy 1pt, medium 2pt, hard 3pt, so clearing the easy '
               'tier alone cannot carry a model. Ranking is on weighted score, not flat pass rate.'
               '<br><br>'
               'Sample is 20 of 89 tasks, drawn proportionally (1 easy / 12 medium / 7 hard) and randomly '
               'within tier, seed 42. At n=20 the 95% interval on a 40% result spans roughly ±22 points, '
               'so small gaps between models are not meaningful — treat this as a first read, not a ranking.'
               '</div>')
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--jobs", type=pathlib.Path, default=JOBS)
    ap.add_argument("--tasks", type=pathlib.Path, default=TASKS)
    ap.add_argument("--suite", default="strat20",
                    help="suite name, or a comma-separated set to union into one "
                         "table (e.g. domain20,domain30-delta)")
    ap.add_argument("--suite-label", help="heading to use when --suite is a set")
    ap.add_argument("--html", type=pathlib.Path)
    a = ap.parse_args()

    if not a.jobs.is_dir():
        sys.exit(f"no jobs directory: {a.jobs}")
    suites = {s.strip() for s in a.suite.split(",") if s.strip()}
    label = a.suite_label or (a.suite if len(suites) == 1 else " + ".join(sorted(suites)))
    meta = task_meta(a.tasks)
    runs = read_jobs(a.jobs, meta)
    summary = summarize(runs, suites)
    if not summary:
        sys.exit(f"no completed runs for suite {a.suite}")

    print(md_table(summary, label))
    if a.html:
        a.html.write_text(html_page(summary, label, runs, meta, suites))
        print(f"\nwrote {a.html}", file=sys.stderr)


if __name__ == "__main__":
    main()
