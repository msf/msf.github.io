+++
title = "DEBS 2026: DuneCP, an event-driven control plane for DuneSQL"
date = "2026-07-24"
description = "Reflections on my DuneCP keynote at DEBS 2026, and why maintenance belongs inside system design."
+++

I recently gave a [keynote at DEBS 2026](https://2026.debs.org/keynote-speakers/#keynote4) about the work we did at Dune during the first half of 2026. The talk was about DuneCP, an event-driven control plane for DuneSQL.

The problem was a missing center. Several areas of operating and maintaining DuneSQL had no shared control plane. The work accumulated in separate services and, too often, in "just another cron job."

## Operations are part of the system

The point I most wanted to make was broader than DuneCP:

**We have a tendency not to build autonomous systems to maintain and operate the systems we build.**

We treat operation and maintenance as work outside the system, left to humans and ad hoc automation. But they are integral parts of the system. If a data platform needs continuous health checks, cleanup, optimization, metering, and recovery, those can be seen as control plane concerns. They are part of the platform.

My focus with DuneCP was deliberately narrower than "centralize everything." It centralizes *some* of this work behind one event model and one set of operational primitives. The more important effect is critical mass. Once enough tasks share one home, the natural place for the next operational task is the control plane, not a new cron job.

One part I really like is the simplicity of the event model and "we just need pgSQL and very few APIs". Always "keep it simple" and obcessively avoid complexity.

## An experiment in making the talk

I didn't use Google Slides. I wrote and composed the deck in Markdown, using LLMs throughout editing and composition. Markdown was the source of truth; the PDF was generated from it.

## What did not land as well as I wanted

I did not explain Dune itself well enough. In particular, I failed to make the complexity of the ingestion pipeline, the breadth of the feature surface, and the scale of the platform concrete: the number of tables and the amount of data behind DuneSQL.

That context matters. Without it, the operational surface can look like a collection of small jobs rather than the second system required to keep a large data platform healthy.

- 📄 [Slides (PDF)](/talks/debs-2026-dunecp.pdf)
- 📅 [DEBS 2026 keynote page](https://2026.debs.org/keynote-speakers/#keynote4)
