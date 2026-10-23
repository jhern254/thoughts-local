# Day activity navigator

The day-activity plot sits below Mindset with quiet space above it, and immediately
above the Events divider. Its marker follows the accepted Events date. It is
rendered by Go/Bubble Tea in native terminals and the browser alike.

Tab order is Mindset → Days → Events → entities. While Days has focus:

- Left/Right (or h/l) moves exactly one day, including ordinary key repeats.
  A pending destination accumulates subsequent keys; stale reads cannot replace it.
- `+`/`-` zooms by approximately 20%; `0` restores the full history range.
  A zoomed range stays still until the selected date leaves it.
- `1`, `2`, `3` toggle Total, Events, Thoughts. Unfocused Days shows only Total;
  returning restores the chosen toggles. First focus shows all three.
- `d` opens local distribution controls: Left/Right changes boost, Up/Down
  changes size, `f` switches Total between filled and outline, `0` resets
  styling. Escape/Enter closes controls. Escape outside controls returns to Events.
- `r` explicitly refreshes activity. These display preferences last for the session.

The plot has no title or date-range heading. Normal-height terminals leave a
blank row above three plot rows. Style values appear there only while their
controls are open; loading/error feedback remains available. Short windows use a
single plot row so the timeline remains usable; the accepted date stays in the
Events heading and controls remain in the footer. Very small terminals retain
the existing resize guidance. No custom key acceleration is included.

## Counts and rendering

Full history starts at the earliest active Event or observed Thought, including
records before account creation, and ends today. Future observations and deleted
records/users do not count. A completed Event contributes once to every calendar
day it intersects, using half-open intervals; a zero-duration Event contributes
to its start day. Ongoing Events stop at the read's cutoff. All observed Thoughts
count once, including unassigned ones. Calendar boundaries use the application's
Pacific timezone and civil dates, including 23/25-hour DST days.

There are at most 256 buckets, also bounded by terminal width and calendar days.
A wider bucket reports average daily counts, not totals that grow solely because
of zooming out.
Each channel is normalized against its own maximum in the visible range:

    Event level   = 0.7 × (Event daily average / maximum Event average)^boost
    Thought level = 0.3 × (Thought daily average / maximum Thought average)^boost
    Total         = Event level + Thought level

Zero channels remain zero. The fixed weights give Events greater visual weight;
there is no user-facing weight setting. Boost presets use exponents
1, 0.8 (default), 0.5, 0.25, 0.15. Size ranges from 50–125%, with headroom rather
than clipping. These are relative activity shapes, not probabilities or raw-count
axes. Piecewise linear interpolation connects samples without invented peaks.

Total is a dim background; Events and Thoughts use teal and amber traces.
Hiding a channel does not renormalize the others. A Braille cell has one color,
so intersecting traces can share that color; the stronger component wins, with
Events breaking ties. Filled Total dots in the same cell inherit that color too.

## Ownership and cost

`internal/data/sqlite_day_activity.go` streams compact Event endpoints once and
counts Thoughts over disjoint indexed ranges. Difference increments account for
long Events without expanding every Event into every historical date. Output is
bounded, but reading all history still scales with the number of stored records.
There is no new schema, index, dependency, background worker, or query per key
inside a loaded range.

The existing metrics service exposes these read projections. `tui/daynav` owns
range, selection, style, and one cancellable read. It retains accepted geometry
while loading, checks owner/request identity, queues invalidation during a read,
and requires explicit retry after failure. Accepted Event mutations, Thought
changes, return/reload and calendar rollover refresh the overview. Resizing
resamples to the new bounded width. Focus/style changes reuse the loaded values;
ordinary selection changes reuse the rendered curve as well.

Events still owns daily loading, cards, inline expansion and selection. Mindset
keeps its full-text reading and scrolling behavior. The plot adds no timers,
WebSocket protocols or browser-only drawing.

## Verification and measurements

Deterministic tests cover weighting and literal Braille geometry; empty activity;
focus/toggle/style behavior; zoom and stale/canceled reads; failed-read retry;
root key routing and detail return; real migrated SQLite ownership, overlap,
zero-duration, observed dates, deleted records, both DST transitions and limits.
The browser regression exercises the same keys through xterm in Chromium and
Firefox. Synthetic visual captures are kept outside the repository.

For hands-on review, use the separate varied-history demo:

```sh
THOUGHTS_BROWSER_PORT=7878 make tui/browser/demo/activity
# Native terminal:
make tui/demo/activity
```

It creates a disposable two-year diary with quiet gaps, Event-heavy and
Thought-heavy periods, crowded days, spanning/zero-duration Events, and an
ongoing Event with 200 Thoughts. Relative timestamps keep it useful on launch.
Zooming out exercises multi-day averages; zooming in reveals individual dates.
The original `tui/browser/demo/stress` fixture remains unchanged for comparable
performance measurements. The varied demo is a visual/navigation workload,
not a replacement benchmark baseline.

A standalone renderer comparison is available with:

```sh
go run ./cmd/render-preview -scenario day-activity -width 80
```

Measured on 2026-10-23, Intel i5-8250U (8 logical CPUs), Linux amd64,
Go 1.25.14. Before is merged PR #88 (`99d791b`, binary built from its
`97b1bf0` base plus the merged bounded-query change); after is this feature's
working tree before rebasing the independently merged full-text Mindset fix.
The rebase preserves that fix and the activity implementation. Measurements
below are revision-specific evidence, not a claim of speed improvement.
Runs used no competing builds/tests. Five repetitions do not establish
statistical significance.

The unchanged synthetic stress fixture has 721 Events and 20,000 Thoughts,
anchored at Unix 1792739869. Both widths yield daily buckets for its roughly
one-month history. Setup, migration and compilation are excluded. Medians of
five one-second runs (`BenchmarkDayActivity`, `-benchmem`):

| Operation | Width | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Activity read (both count queries) | 80 | 5,712,009 | 30,635 | 2,297 |
| Loaded selection + View + marker | 80 | 2,664 | 1,056 | 7 |
| Activity read (both count queries) | 160 | 5,503,443 | 30,608 | 2,297 |
| Loaded selection + View + marker | 160 | 2,882 | 1,696 | 7 |

Existing `BenchmarkTimeline` smoke comparison, median of five one-second runs.
These measure the daily renderer, not the whole root view or the new plot:

| Size / operation | Before ns/op | After ns/op | B/op before → after | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 80×27 Render | 898,003 | 903,498 | 93,315 → 93,409 | 2,321 |
| 80×27 Navigate | 1,378,846 | 1,365,722 | 156,892 → 156,718 | 3,812 |
| 80×27 Expanded | 905,822 | 930,268 | 131,704 → 132,082 | 3,143 |
| 40×20 Render | 533,630 | 555,969 | 80,747 → 80,634 | 2,108 |
| 40×20 Navigate | 975,108 | 968,815 | 143,244 → 143,588 | 3,483 |
| 40×20 Expanded | 683,300 | 697,952 | 91,206 → 90,973 | 2,465 |

Headful Brave 148.0.7778.217, 1200×850, static dark background, same database:
five groups of 12 alternating historical day keys through the real WebSocket.
Latency starts at keydown and ends at xterm's render with the requested date and
its settled 30-Thought count. These are whole-app observations:

| Metric | Before | After |
| --- | ---: | ---: |
| Median key-to-display | 42.3 ms | 41.45 ms |
| p95 key-to-display | 47.3 ms | 54.0 ms |
| Native CPU for 60 transitions | 2.37 s | 2.64 s |
| Browser process-tree CPU | 2.09 s | 2.30 s |
| Native RSS after workload | 41,892 KiB | 45,792 KiB |

The p95 increased 6.7 ms and sampled CPU increased about 11%; this is additional
presentation work, not a performance improvement. RSS is a single observation,
not retained-allocation evidence. No missed keys occurred in this workload.
Video, hardware acceleration and the unresolved black-rectangle report are not
retested or claimed fixed by these static-background measurements.
