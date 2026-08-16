#!/usr/bin/env python3
"""Generate the SVG charts embedded in the exam_v4 blog post.

Output lands in static/images/exam-v4/ and is committed, so the post renders
without a build step. Re-run after changing any number below; the data here is
transcribed from docs/reports/EXAM_V3_2026-08-07_R9700.md and
docs/reports/EXAM_V4_2026-08-09_TB21.md and
docs/reports/EXAM_V4_2026-08-12_MUSE_GLIMMER.md.

Palette is validated against the site's dark surface (#181818) with the dataviz
skill's validator: categorical #3d8fc8 / #c08417, ordinal blue ramp
#2a6b96 -> #3d8fc8 -> #7cc0ea. Do not swap colours without re-running it.
"""

from pathlib import Path

OUT = Path(__file__).resolve().parents[3] / "static" / "images" / "exam-v4"

FG = "#d8d8d8"       # primary ink
MUTED = "#a8a8a8"    # secondary ink
GRID = "#3a3a3a"
BLUE = "#3d8fc8"     # categorical 1 / single series
AMBER = "#c08417"    # categorical 2
TIER = {"easy": "#2a6b96", "medium": "#3d8fc8", "hard": "#7cc0ea"}
SURFACE = "#181818"  # only used for the 2px separation ring between marks

FONT = ('font-family="-apple-system,BlinkMacSystemFont,\'Segoe UI\','
        'Roboto,Helvetica,Arial,sans-serif"')

W = 760
PAD_L = 208
PAD_R = 40


def svg(height, body, title):
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {height}" '
        f'width="100%" style="max-width:{W}px;height:auto" role="img" '
        f'aria-label="{title}" {FONT}>\n'
        f'<title>{title}</title>\n{body}</svg>\n'
    )


def text(x, y, s, size=13, fill=FG, anchor="start", weight="400", opacity=1.0):
    return (f'<text x="{x:.1f}" y="{y:.1f}" font-size="{size}" fill="{fill}" '
            f'text-anchor="{anchor}" font-weight="{weight}" '
            f'opacity="{opacity}">{s}</text>\n')


def bar(x, y, w, h, fill, r=4, round_left=False, round_right=True):
    """Bar with rounded data-end only; the baseline end stays square."""
    w = max(w, 0.0)
    r = min(r, w / 2, h / 2)
    if r <= 0:
        return f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h}" fill="{fill}"/>\n'
    rl, rr = (r if round_left else 0), (r if round_right else 0)
    d = (f'M{x + rl:.1f},{y} H{x + w - rr:.1f} '
         f'{f"a{rr},{rr} 0 0 1 {rr},{rr} " if rr else ""}'
         f'V{y + h - rr:.1f} '
         f'{f"a{rr},{rr} 0 0 1 -{rr},{rr} " if rr else ""}'
         f'H{x + rl:.1f} '
         f'{f"a{rl},{rl} 0 0 1 -{rl},-{rl} " if rl else ""}'
         f'V{y + rl:.1f} '
         f'{f"a{rl},{rl} 0 0 1 {rl},-{rl} " if rl else ""}Z')
    return f'<path d="{d}" fill="{fill}"/>\n'


INSET = 4  # keeps glyph overshoot off the viewBox edge


def header(title, subtitle):
    s = text(INSET, 16, title, size=15, weight="600")
    if subtitle:
        s += text(INSET, 34, subtitle, size=12, fill=MUTED)
    return s


# ── 1. exam_v3 seed spread on the R9700 ──────────────────────────────────────
def chart_seed_variance():
    rows = [
        ("Gemma 4 31B QAT", [11, 12, 12, 12, 5], 12),
        ("Muse Glimmer 30B", [5, 0, 11, 12, 8], 8),
        ("Qwen3.6 35B-A3B MoE + MTP", [6, 11, 6, 7, 10], 7),
        ("Qwen3.8 27B + MTP", [7, 0, 6, 6, 7], 6),
        ("Gemma 4 31B PTQ", [11, 5, 12, 5, 5], 5),
        ("Gemma 4 26B-A4B MoE", [7, 0, 5, 11, 5], 5),
        ("Qwen3.6 27B + MTP", [0, 6, 6, 0, 0], 0),
        ("Gemma 4 E4B", [0, 0, 9, 0, 0], 0),
    ]
    top, row_h = 66, 52
    height = top + len(rows) * row_h + 46
    x0, x1 = PAD_L + 10, W - PAD_R
    sx = lambda v: x0 + (x1 - x0) * v / 13.0
    y_bot = top + len(rows) * row_h - 18

    b = header("exam_v3 on the R9700: five seeds per model",
               "one dot per seed, 13-point exam · amber marker = median")

    for v in range(0, 13, 2):
        if v == 12:  # the ceiling rule owns this position
            continue
        b += (f'<line x1="{sx(v):.1f}" y1="{top - 12}" x2="{sx(v):.1f}" '
              f'y2="{y_bot}" stroke="{GRID}" stroke-width="1"/>\n')
    for v in range(0, 14, 2):
        b += text(sx(v), y_bot + 18, str(v), size=11, fill=MUTED, anchor="middle")
    b += text(sx(6.5), y_bot + 40, "score / 13", size=12, fill=MUTED, anchor="middle")

    ceiling = sx(12)
    b += (f'<line x1="{ceiling:.1f}" y1="{top - 12}" x2="{ceiling:.1f}" '
          f'y2="{y_bot}" stroke="{AMBER}" stroke-width="1" stroke-dasharray="4 3" '
          f'opacity="0.8"/>\n')

    for i, (name, scores, med) in enumerate(rows):
        cy = top + i * row_h + 10
        b += text(x0 - 24, cy + 4, name, size=13, anchor="end")
        # spread rule: min→max, so the range reads before the individual dots
        b += (f'<line x1="{sx(min(scores)):.1f}" y1="{cy:.1f}" '
              f'x2="{sx(max(scores)):.1f}" y2="{cy:.1f}" stroke="{GRID}" '
              f'stroke-width="2"/>\n')
        counts = {}
        for v in sorted(scores):
            k = counts.get(v, 0)
            counts[v] = k + 1
            # stack repeats vertically so ties stay countable
            dy = (k - (scores.count(v) - 1) / 2) * 11
            b += (f'<circle cx="{sx(v):.1f}" cy="{cy + dy:.1f}" r="5" fill="{BLUE}" '
                  f'stroke="{SURFACE}" stroke-width="2"/>\n')
        # median sits below the stack where nothing can occlude it
        mxp, myp = sx(med), cy + 26
        b += (f'<path d="M{mxp:.1f},{myp - 7:.1f} L{mxp + 5:.1f},{myp:.1f} '
              f'L{mxp - 5:.1f},{myp:.1f} Z" fill="{AMBER}"/>\n')

    b += text(x1, y_bot + 40, "dashed → effective ceiling, 12", size=11,
              fill=MUTED, anchor="end")
    return svg(height, b, "exam_v3 scores on the R9700, five seeds per model")


# ── 2. Framework 13 vs R9700 decode throughput ───────────────────────────────
def chart_throughput():
    rows = [
        ("Framework 13 · Radeon 890M", "Qwen 35B-A3B MoE + MTP", 11.6, AMBER),
        ("R9700 · same build", "Qwen 35B-A3B MoE + MTP", 126.7, BLUE),
        ("R9700 · dense", "Gemma 4 31B QAT, no drafter", 24.5, BLUE),
        ("R9700 · dense + MTP", "Gemma 4 31B QAT, drafter n=4", 56.3, BLUE),
    ]
    top, row_h, bh = 72, 46, 20
    height = top + len(rows) * row_h + 30
    x0, x1 = PAD_L, W - PAD_R - 46
    mx = 130.0
    sx = lambda v: (x1 - x0) * v / mx

    b = header("Decode throughput, identical GGUF where marked",
               "tokens/s at ~10k generated tokens, reasoning on")
    b += (f'<rect x="{INSET}" y="46" width="10" height="10" rx="2" fill="{AMBER}"/>\n'
          + text(INSET + 16, 55, "Framework 13", size=12, fill=MUTED)
          + f'<rect x="{INSET + 112}" y="46" width="10" height="10" rx="2" fill="{BLUE}"/>\n'
          + text(INSET + 128, 55, "hopper / R9700", size=12, fill=MUTED))

    for i, (name, sub, v, colour) in enumerate(rows):
        y = top + i * row_h
        b += text(x0 - 14, y + 10, name, size=13, anchor="end")
        b += text(x0 - 14, y + 25, sub, size=11, fill=MUTED, anchor="end")
        b += bar(x0, y, sx(v), bh, colour)
        b += text(x0 + sx(v) + 10, y + 15, f"{v:g}", size=13, weight="600")

    b += text(INSET, height - 8, "10.9× on the identical MoE build; both machines "
              "measured under powersave", size=11, fill=MUTED)
    return svg(height, b, "Decode throughput, Framework 13 versus R9700")


# ── 3. exam_v4 weighted score ────────────────────────────────────────────────
def chart_scores():
    rows = [
        ("qwen-35b-moe-mtp", 1, 5, 2),
        ("qwen-27b-mtp", 1, 5, 1),
        ("gemma-31b-qat", 1, 4, 1),
        ("muse-glimmer-30b", 1, 3, 1),
        ("qwen-27b-mtp-q6", 1, 4, 0),
        ("gemma-26b-moe", 1, 3, 0),
    ]
    top, row_h, bh = 76, 40, 20
    height = top + len(rows) * row_h + 30
    x0, x1 = PAD_L, W - PAD_R - 74
    sx = lambda v: (x1 - x0) * v / 46.0

    b = header("exam_v4: weighted score on strat20",
               "20 Terminal-Bench 2.1 tasks, one attempt each · easy 1, "
               "medium 2, hard 3 · max 46")
    lx = INSET
    for tier, label in (("easy", "easy"), ("medium", "medium"), ("hard", "hard")):
        b += f'<rect x="{lx}" y="46" width="10" height="10" rx="2" fill="{TIER[tier]}"/>\n'
        b += text(lx + 16, 55, label, size=12, fill=MUTED)
        lx += 78

    for i, (name, e, m, h) in enumerate(rows):
        y = top + i * row_h
        pts = [("easy", e * 1), ("medium", m * 2), ("hard", h * 3)]
        total = sum(p for _, p in pts)
        b += text(x0 - 14, y + 15, name, size=13, anchor="end")
        b += (f'<rect x="{x0}" y="{y}" width="{sx(46):.1f}" height="{bh}" rx="4" '
              f'fill="{GRID}" opacity="0.5"/>\n')
        cx = x0
        drawn = [(t, p) for t, p in pts if p > 0]
        for j, (tier, p) in enumerate(drawn):
            w = sx(p) - (2 if j < len(drawn) - 1 else 0)  # 2px surface gap
            b += bar(cx, y, w, bh, TIER[tier],
                     round_left=(j == 0), round_right=(j == len(drawn) - 1))
            cx += sx(p)
        b += text(x0 + sx(46) + 12, y + 15,
                  f"{total}/46 ({round(100 * total / 46)}%)", size=13, weight="600")

    b += text(INSET, height - 8, "tasks solved: 8, 7, 6, 5, 5, 4 of 20", size=11, fill=MUTED)
    return svg(height, b, "exam_v4 weighted scores by difficulty tier")


# ── 4. wall-clock cost ───────────────────────────────────────────────────────
def chart_runtime():
    rows = [
        ("qwen-35b-moe-mtp", 6.13, "6h08m", 8),
        ("qwen-27b-mtp", 7.25, "7h15m", 7),
        ("gemma-31b-qat", 7.60, "7h36m", 6),
        ("muse-glimmer-30b", 6.70, "6h42m", 5),
        ("qwen-27b-mtp-q6", 7.22, "7h13m", 5),
        ("gemma-26b-moe", 8.17, "8h10m", 4),
    ]
    top, row_h, bh = 76, 40, 20
    height = top + len(rows) * row_h + 32
    x0, x1 = PAD_L, W - PAD_R - 96
    mx = 9.0
    sx = lambda v: (x1 - x0) * v / mx

    b = header("What 20 tasks cost in wall-clock time",
               "one attempt per task, one trial at a time · 43h06m total")
    b += text(INSET, 55, "more failures → more time: a failed task burns its whole "
              "agent timeout", size=12, fill=MUTED)

    for i, (name, h, label, solved) in enumerate(rows):
        y = top + i * row_h
        b += text(x0 - 14, y + 15, name, size=13, anchor="end")
        b += bar(x0, y, sx(h), bh, BLUE)
        b += text(x0 + sx(h) + 10, y + 15, f"{label}  ·  {solved}/20 solved",
                  size=13, fill=FG)

    b += text(INSET, height - 8, "the full 89-task suite extrapolates to ~33h per "
              "model — over a week of GPU time for six", size=11, fill=MUTED)
    return svg(height, b, "Wall-clock runtime per model on the 20-task subset")


# ── 5. per-model throughput observed during the exam runs ────────────────────
def chart_throughput_by_model():
    """Decode and prefill throughput per model, from the monitoring sidecar.

    Two panels rather than one chart with two y-scales: tg/s and pp/s differ by
    roughly 10x, and a dual axis would invent a relationship between them. Each
    panel carries its own scale and its own hue; row order is identical in both
    so a model can be tracked down the figure.

    Numbers are means over the exam windows, so they are rough - real agentic
    load with prompt-cache hits and swaps in it, not a controlled bench.
    `gemma-e4b` is in the dashboard but was never an exam_v4 arm, so it is left
    out here; `qwen-27b-mtp-q6` ran before the sidecar was scraping it.
    """
    decode = [
        ("qwen-35b-moe-mtp", 118.0),
        ("gemma-26b-moe", 110.0),
        ("qwen-27b-mtp", 49.0),
        ("gemma-31b-qat", 46.4),
        ("muse-glimmer-30b", 42.3),
    ]
    prefill = [
        ("qwen-35b-moe-mtp", 1256),
        ("gemma-26b-moe", 949),
        ("qwen-27b-mtp", 394),
        ("gemma-31b-qat", 365),
        ("muse-glimmer-30b", 345),
    ]
    row_h, bh = 34, 18
    panel_gap = 30
    top_a = 74
    top_b = top_a + len(decode) * row_h + panel_gap
    height = top_b + len(prefill) * row_h + 26
    x0, x1 = PAD_L, W - PAD_R - 58

    b = header("Throughput per model during the exam runs",
               "rough pp/s and tg/s captured from the monitoring sidecar "
               "while the exams executed")

    def panel(y_label, title, rows, mx, colour, fmt):
        s = text(INSET, y_label, title, size=13, weight="600")
        sx = lambda v: (x1 - x0) * v / mx
        for i, (name, v) in enumerate(rows):
            y = y_label + 14 + i * row_h
            s_ = text(x0 - 14, y + 13, name, size=13, anchor="end")
            s_ += bar(x0, y, sx(v), bh, colour)
            s_ += text(x0 + sx(v) + 10, y + 13, fmt(v), size=13, weight="600")
            s += s_
        return s

    b += panel(top_a - 14, "Decode  ·  tokens/s", decode, 130.0, BLUE,
               lambda v: f"{v:g}")
    b += panel(top_b - 14, "Prefill  ·  prompt tokens/s", prefill, 1400.0, AMBER,
               lambda v: f"{v:,}")

    b += text(INSET, height - 6,
              "means over each run window · MoE models decode and prefill far "
              "faster than the dense ones", size=11, fill=MUTED)
    return svg(height, b, "Decode and prefill throughput per model during the "
                          "exam_v4 runs")


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for name, fn in (("exam-v3-seed-variance", chart_seed_variance),
                     ("throughput-fw13-vs-r9700", chart_throughput),
                     ("exam-v4-scores", chart_scores),
                     ("exam-v4-runtime", chart_runtime),
                     ("exam-v4-throughput-by-model", chart_throughput_by_model)):
        path = OUT / f"{name}.svg"
        path.write_text(fn())
        print(f"wrote {path}")


if __name__ == "__main__":
    main()
