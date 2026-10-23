# Day activity navigator

The day-activity plot sits below Mindset with quiet space above it, and immediately
above the Events divider. Its marker follows the accepted Events date. It is
rendered by Go/Bubble Tea in native terminals and the browser alike.

Up from the first Event enters the day graph; Down returns to Events.
Left/Right day changes retain graph focus. Tab cycles Mindset → Events → entities;
the graph is an arrow-accessed part of Events. While the graph has focus:

- Left/Right (or h/l) moves exactly one day, including ordinary key repeats.
  A pending destination accumulates subsequent keys; stale reads cannot replace it.
- `+`/`-` zooms by approximately 20%; `0` restores the full history range.
  A zoomed range stays still until the selected date leaves it.
- `1`, `2`, `3` toggle Total, Events, Thoughts. Unfocused Days shows only Total;
  returning restores the chosen toggles. First focus shows all three.
- `d` opens local distribution controls: Left/Right changes boost, Up/Down
  changes size, `f` switches Total between filled and outline, `0` resets
  styling. Escape/Enter closes controls. Down or Escape outside controls returns to Events.
- `r` explicitly refreshes activity. These display preferences last for the session.

When the graph is focused, all vertical Event curves are highlighted too;
card selection and borders keep their own focus rules.

The plot has no title or date-range heading. Normal-height terminals leave a
blank row above five plot rows on tall windows (four on medium windows). Style values appear there only while their
controls are open; loading/error feedback remains available. Short windows use a
single plot row so the timeline remains usable; the accepted date stays in the
Events heading and controls remain in the footer. Very small terminals retain
the existing resize guidance. No custom key acceleration is included.

The end-of-day metrics card also accepts Left/Right (h/l) to cycle calendar
days while keeping the card focused. A new day's subject list starts at the top;
Right at today stays put. Up returns to the timeline boundary.

## Counts and rendering

Full history starts at the earliest active Event or observed Thought, including
records before account creation, and ends today. Future observations and deleted
records/users do not count. A completed Event contributes once to every calendar
day it intersects, using half-open intervals; a zero-duration Event contributes
to its start day. Ongoing Events stop at the read's cutoff. All observed Thoughts
count once, including unassigned ones. Calendar boundaries use the application's
Pacific timezone and civil dates, including 23/25-hour DST days.

There are at most 256 buckets, also bounded by calendar days and twice the
terminal width, using both horizontal dots in each Braille cell.
A wider bucket reports average daily counts, not totals that grow solely because
of zooming out.
Each channel is normalized against its own maximum in the visible range:

    Event level   = 0.7 × (Event daily average / maximum Event average)^boost
    Thought level = 0.3 × (Thought daily average / maximum Thought average)^boost
    Total         = Event level + Thought level

Zero channels remain zero. The fixed weights give Events greater visual weight;
there is no user-facing weight setting. Boost presets use exponents
0.8, 1.1 (default), 1.4, 1.8, 2.4. Higher boost deepens valleys and sharpens
summits. Size ranges from 50–300%: it grows the actual plot up to twelve rows,
subject to the window budget, while retaining at least fourteen Events rows.
Small windows therefore cannot display the full requested enlargement.
Amplitude follows `1 - exp(-1.6 × size)`, with size expressed as a ratio: about
80% of the canvas at 100%, approaching 99% at 300%. This avoids clipping large
peaks into artificial flat ridges. These are relative activity shapes, not probabilities or raw-count
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

## Mountain detail and controls

The mountain-style follow-up was compared with `8dbe4cb` on the same i5-8250U,
Go 1.25.14 and Brave 148 reference machine, before the subsequent focus-navigation
changes. Binaries were compiled first; measurements ran sequentially without
competing builds/tests. Five runs describe this machine, not statistical significance.

The original stress fixture remains 721 Events/20,000 Thoughts. The varied
activity fixture has 3,927 Events/29,479 Thoughts across roughly two years.
Both revisions used the same migrated fixture files. The baseline received only
the matched benchmark-test changes. Load includes bounds, counts, and repaint;
Rebuild measures the focused plot; Navigate reuses accepted data and the cached
plot. Default plot height at the measured 40-row window grows from three to five
rows, and long-history sampling grows from one to two samples per terminal cell,
capped at 256. The added work is intentional detail, not an optimization.

Medians of five one-second runs on the long-history fixture:

| Width / operation | Before ns/op | Mountain ns/op | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| 80 Load | 13,082,807 | 13,645,988 | 157,076 → 181,649 | 12,108 → 12,294 |
| 80 Rebuild | 149,313 | 324,757 | 13,648 → 28,264 | 226 → 608 |
| 80 Navigate | 688 | 994 | 896 → 2,048 | 1 → 1 |
| 160 Load | 13,376,908 | 14,672,404 | 184,360 → 218,160 | 12,276 → 12,506 |
| 160 Rebuild | 274,065 | 479,943 | 25,792 → 46,456 | 419 → 798 |
| 160 Navigate | 859 | 1,158 | 1,792 → 3,072 | 1 → 1 |

Load on the original stress fixture changed from 6.15 to 6.17 ms at 80 columns,
and 5.96 to 6.14 ms at 160. Long-history repaint cost more than doubled, but
remains below 0.5 ms in these runs and is not performed on each in-range day key.
The loaded plot's View/marker path remains around one microsecond.

Headful Brave at 1200×850 used the unchanged stress fixture and the focused day
plot. Each condition ran five groups of twelve alternating historical day keys,
waiting for the requested date and settled 30-Thought count in xterm. Dramatic
means 300% size with extra contrast; it uses twelve plot rows on this viewport.

| Metric | Before default | Mountain default | Dramatic |
| --- | ---: | ---: | ---: |
| Median key-to-display | 27.05 ms | 27.30 ms | 34.55 ms |
| p95 key-to-display | 49.30 ms | 43.90 ms | 45.70 ms |
| Native CPU, 60 transitions | 2.08 s | 2.08 s | 2.18 s |
| Browser process-tree CPU | 1.70 s | 1.72 s | 2.12 s |
| Native RSS after workload | 39,680 KiB | 42,708 KiB | 44,204 KiB |

Dramatic rendering uses more CPU and display area. The lower sampled p95 is not
evidence of an improvement; frame scheduling varies. No keys were lost in these
runs. RSS samples do not establish retained allocations. No new timer, animation,
background worker or per-key aggregate query is introduced. These measurements
cover static browser mode, not video playback or the unresolved compositor issue.
