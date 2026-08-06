#!/usr/bin/env python3
"""Build the response a perfect model would have produced for exam_v3.

Splices scraper_solution.go's NewScraper + implementation into scraper.go's
skeleton, replacing the buggy defaultScraper, and wraps the result in the
```go fence the prompt mandates. Everything the skeleton owns -- Metric, the
three interfaces, HTTPSink/HTTPSource, InverterData, collect, kostalPower,
main -- stays intact, which is exactly the substitution scraper_solution.go's
header describes.

Its only purpose is to prove eval.sh still scores a known-good submission
13/13. If it does not, no model score from that run means anything.

    make-reference-response.py <bench-dir> <out-file>
"""

import re
import sys
from pathlib import Path

# The skeleton's NewScraper + defaultScraper region, which the solution replaces.
IMPL_START = "// NewScraper builds the default Scraper implementation."
IMPL_END = "// --- HTTP implementations ---"
SOLUTION_START = "func NewScraper("


def imports_of(src: str) -> set[str]:
    m = re.search(r"^import \(\n(.*?)\n\)\n", src, re.S | re.M)
    if not m:
        return set()
    return {line.strip().strip('"') for line in m.group(1).splitlines() if line.strip()}


def binds_as(path: str) -> str:
    """Import path -> the identifier it binds. math/rand/v2 -> rand."""
    parts = path.split("/")
    if len(parts) > 1 and re.fullmatch(r"v[0-9]+", parts[-1]):
        return parts[-2]
    return parts[-1]


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    bench, out = Path(sys.argv[1]), Path(sys.argv[2])
    skel = (bench / "scraper.go").read_text()
    sol = (bench / "scraper_solution.go").read_text()

    start = skel.find(IMPL_START)
    end = skel.find(IMPL_END)
    sol_start = sol.find(SOLUTION_START)
    if min(start, end, sol_start) < 0:
        print(
            f"splice markers not found (start={start} end={end} sol={sol_start})",
            file=sys.stderr,
        )
        return 1

    merged = skel[:start] + sol[sol_start:].rstrip() + "\n\n" + skel[end:]

    # Union of both import blocks, minus anything the merged body no longer uses.
    keep = sorted(
        p for p in imports_of(skel) | imports_of(sol)
        if re.search(r"\b%s\." % re.escape(binds_as(p)), merged)
    )
    merged = re.sub(
        r"^import \(\n.*?\n\)\n",
        "import (\n" + "\n".join('\t"%s"' % p for p in keep) + "\n)\n",
        merged,
        count=1,
        flags=re.S | re.M,
    )

    out.write_text("```go\n" + merged.rstrip() + "\n```\n")
    print(f"wrote {out} ({len(merged)} bytes; imports: {' '.join(keep)})", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
