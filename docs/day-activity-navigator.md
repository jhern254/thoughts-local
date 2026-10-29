# Day activity navigator

The day-activity plot sits below Mindset with quiet space above it, and immediately
above the Events heading. The mountains share a Braille baseline. A solid purple block in the gap beneath
them marks the accepted Events date, using the Events highlight color. It is
rendered by Go/Bubble Tea in native terminals and the browser alike.

Up from the first Event enters the day graph; Down returns to Events.
Left/Right day changes retain graph focus. Tab cycles Mindset → Events → entities;
the graph is an arrow-accessed part of Events. While the graph has focus:

- Left/Right (or h/l) moves exactly one day, including ordinary key repeats.
  A pending destination accumulates subsequent keys; stale reads cannot replace it.
- `+`/`-` zooms by approximately 20%; `0` restores the default calendar-month
  zoom ending on the selected date. These keys also work while Events, expanded
  Thoughts, or its metrics card retains focus. Forms and distribution controls
  keep their own input/reset behavior. Zoom-out fits the available history,
  with at least the default month so a short history cannot shrink the view.
  The default runs from one calendar month ago through today (October 23 starts
  at September 23). Short months clamp to their last valid date. The range stays
  still until the selected date leaves it.
- `1`, `2`, `3` toggle Total, Events, Thoughts. All three are visible initially;
  chosen toggles persist. The plot is grey while Events has focus, and colored
  in the day graph, Mindset, and entity picker.
- `d` opens local distribution controls: Left/Right changes boost, Up/Down
  changes size, `f` switches the mountains between filled and outline, `0` resets
  styling. Escape/Enter closes controls. Down or Escape outside controls returns to Events.
- `r` explicitly refreshes activity. These display preferences last for the session.

When focus is outside the Events timeline, all vertical Event curves are highlighted too;
card selection and borders keep their own focus rules.

The plot has no title or date-range heading. All zoom levels use at most four mountain rows.
The purple marker has its own row beneath that budget. Short terminals reduce
mountain rows before taking Events below fourteen rows when space permits:
26 terminal rows use three mountain rows, 25 use two, and 24 or fewer use one.
Size/boost tuning never grows this canvas. Resize updates the Events allocation in the same root-model transition. Zoom
never enlarges the graph. One blank row separates
Mindset from the graph; asynchronous reminders do not change its height budget.

Style values, loading feedback, and read errors appear right-aligned beside the
Events date, with at least two separating columns. If both cannot fit, the status
uses the existing first footer row; keyboard help remains on the second. Status
never replaces a mountain row. As in Events, the loading label waits 150 ms;
reads and accepted data are never delayed. Completion, cancellation, hiding the
graph, and replacement reset feedback. Request ownership rejects retired timers,
so a queued pan starts a fresh delay rather than inheriting a loading label. Very small terminals retain the existing resize
guidance. No custom key acceleration is included.

The Now/day-boundary view retains its latest card, boundary bar and metrics
preview while Left/Right (h/l) cycles days, including empty days. Returning to
today resumes its live Now view; Right at today stays there. Entry browsing still
opens a changed day at its first card. The metrics card similarly keeps its own
focus while cycling dates.

Mindset also accepts Left/Right (h/l) for day changes without leaving the panel.
It reloads that day's available Thoughts; Up/Down and paging keep their reading
behavior. Only Enter opens the selected Mindset Thought. The entity strip keeps
its existing horizontal entity navigation.

## Counts and rendering

Full history starts at the earliest active Event or observed Thought, including
records before account creation, and ends today. Future observations and deleted
records/users do not count. A completed Event contributes once to every calendar
day it intersects, using half-open intervals; a zero-duration Event contributes
to its start day. Ongoing Events stop at the read's cutoff. All observed Thoughts
count once, including unassigned ones. Calendar boundaries use the application's
Pacific timezone and civil dates, including 23/25-hour DST days.

Visible buckets are whole civil-day intervals anchored to a fixed calendar epoch.
They target twice the terminal width; neighboring context and a partly visible
edge bucket keep each query within 256 buckets. Context continues clipped flanks
without widening the viewport. A wider bucket reports average daily counts,
not totals that grow solely because of zooming out. The same dated bucket keeps
its contents and position as the viewport scrolls.

All channels share the maximum weighted total across the full active history at
the current bucket resolution. This reference remains fixed during panning,
even when the largest peak leaves or enters the viewport:

    Event activity   = 0.7 × Event daily average
    Thought activity = 0.3 × Thought daily average
    Combined         = Event activity + Thought activity
    Total level      = (Combined / maximum Combined)^boost
    Event level      = 0.75 × Total level × Event activity / Combined
    Thought level    = 0.75 × Total level × Thought activity / Combined

Zero combined activity produces zero levels. An Event retains greater per-record
weight, while many Thoughts naturally outweigh fewer Events. All three use the same weighted reference. A shared 0.75 component-height
factor reserves at least a quarter of Total's height above either component;
this is display separation, not a change to the underlying counts. The ratio
between Events and Thoughts still follows the data rather than a forced order.

Zero channels remain zero. The fixed weights give Events greater visual weight;
there is no user-facing weight setting. Boost presets use exponents
0.5 (low), 0.8 (normal, default), 1.1 (medium), 1.8 (strong), 2.4 (extra). Higher boost deepens valleys and sharpens
summits. Size ranges from 50–300%; startup and Reset use normal boost at 100%.
That baseline preserves the former full mountain profile within the compact
bounded canvas. Below 100%, size reduces height proportionally. Above 100%,
smaller ridges rise toward the reference summit without clipping it flat:
`height = size × height / (1 + (size - 1) × height)`. All components receive the
same gain, retaining their count ratios and Total headroom. At 100%, the gain is
one. Higher settings change the picture, not the panel height.

The four-row canvas stays fixed while tuning and zooming, including one-day
zoom. Normal and quarter-size windows keep the same height at 27 terminal rows
or more. Shorter terminals give rows back to Events: 26 rows use three mountain
rows, 25 use two, and 24 or fewer use one. This protects fourteen Events rows
where space permits; it is not scaling with browser width. Counts remain daily
averages, so zoom cannot reveal real intraday peaks.

The renderer takes the maximum of asymmetric finite-support mountain envelopes
rather than adding their heights. The selected B contour blends 50% raised cosine with a 50%
linear summit contribution, separating nearby crests without removing shoulders.
Calendar-anchored supports range from 1.8–2.7 buckets left and 1.2–1.8 right. Two small
ridge scales (1.6 and 0.65 buckets) lower the envelope by at most 2–4%, vanishing
at its measured summit. These are visual interpretations around actual activity,
not invented intraday measurements or unrelated decorative peaks. No activity
means only the optional baseline. Broad interpretation can hide small nearby
peaks and round sparse days into neighboring empty days, within finite support.

The navigator reads six whole context buckets on each side, reducing the visible
bucket budget as needed to keep each query at 256 or fewer buckets. Existing
history-scale refresh, cancellation, calendar alignment and stale-reply checks
remain authoritative. Panning translates the same absolute Braille grid;
outline continuity includes the preceding dot. Selecting a date within the
loaded window changes only the marker and reuses the cached mountain string.
Daily counts and terrain geometry remain separate concrete values; future
activity channels can extend that boundary without changing stored counts now.

Filled mode uses opaque silhouettes: grey Total behind amber Thoughts behind
teal Events, each filled to the baseline. Each shared Braille cell takes the color with the largest exposed dot area
after foreground occlusion; ties favor the front layer. This keeps tiny foreground
contributions from recoloring a whole compact cell. One cell supports one
foreground color, so sub-cell boundaries are quantized. Event-heavy days can
naturally obscure some Thoughts. Outline mode retains connected contour strokes
and blank interiors. Hiding a channel does not renormalize the others.


## Ownership and cost

`internal/data/sqlite_day_activity.go` streams compact Event endpoints once and
counts Thoughts over disjoint indexed ranges. Difference increments account for
long Events without expanding every Event into every historical date. Output is
bounded. Establishing a new scale reads history in batches of at most 256
buckets, retaining only the maximum. Its work scales with history length and
stored records, and remains cancellable between batches and inside SQLite.
This occurs on initial loading, invalidation/refresh, or a changed bucket
resolution. A viewport pan reuses the accepted reference and reads only its
bounded visible range plus neighbors. There is no complete-history read per
ordinary day key or pan.
There is no new schema, index, dependency, background worker, or query per key
inside a loaded range.

The existing metrics service exposes these read projections. `tui/daynav` owns
range, selection, style, and one cancellable read. It retains accepted geometry
while loading, checks owner/request identity, queues invalidation during a read,
and requires explicit retry after failure. Accepted Event mutations, Thought
changes, return/reload and calendar rollover refresh the overview. Resizing
resamples to the new bounded width; a changed bucket resolution refreshes its
historical scale. Focus/style changes reuse the loaded values;
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
performance measurements. Its recent month also has varied samples for the closer default view.
The current seed has 3,992 Events and 30,493 Thoughts. The varied demo is a visual/navigation workload,
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

The earlier mountain-style follow-up was compared with `8dbe4cb` on the same i5-8250U,
Go 1.25.14 and Brave 148 reference machine, before the subsequent focus-navigation
changes. Binaries were compiled first; measurements ran sequentially without
competing builds/tests. Five runs describe this machine, not statistical significance.

The original stress fixture remains 721 Events/20,000 Thoughts. The then-current
varied activity fixture had 3,927 Events/29,479 Thoughts across roughly two years.
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

## Bounded month-view follow-up

Historical comparison: `4beacc1` versus the working tree committed as `1a07bb1`,
before the compact-height correction below, on the same reference
machine/toolchain/browser, using the unchanged 721-Event/20,000-Thought stress
fixture and anchor. That revision's default is low boost, the former 300% look mapped
to 100%, and one calendar month of activity. The canvas is capped at twelve rows;
tuning does not move Events. Comparisons ran sequentially with binaries compiled
first and no competing builds/tests. Five runs are descriptive, not statistical
significance or a speedup claim.

Medians of five one-second native runs at a 40-row terminal:

| Width / operation | Before ns/op | After ns/op | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| 80 Load | 6,245,150 | 5,906,500 | 51,077 → 73,679 | 2,409 → 2,594 |
| 80 Rebuild | 104,486 | 276,462 | 16,560 → 38,824 | 85 → 244 |
| 80 Navigate | 687 | 1,372 | 1,152 → 3,200 | 1 → 1 |
| 160 Load | 6,155,958 | 5,683,206 | 64,108 → 110,567 | 2,413 → 2,609 |
| 160 Rebuild | 188,943 | 444,462 | 29,904 → 75,808 | 110 → 259 |
| 160 Navigate | 938 | 1,480 | 2,304 → 6,144 | 1 → 1 |

The larger default canvas costs about 2.4–2.6 times as much to rebuild here,
remaining below 0.45 ms. Copying the cached View also costs more. Rebuilds do not
run for each in-range day key; palette changes and real layout/style/data changes
rebuild without adding database reads. History Load remains about six milliseconds
on this fixture. No performance improvement is claimed.

Actual headful Brave at 1200×850, static dark background, five groups of twelve
alternating historical day keys, waiting for the requested date and settled
30-Thought count in xterm:

| Metric | Before default | New default |
| --- | ---: | ---: |
| Median key-to-display | 42.10 ms | 41.85 ms |
| p95 key-to-display | 49.10 ms | 56.90 ms |
| Native CPU, 60 transitions | 2.35 s | 2.58 s |
| Browser process-tree CPU | 1.86 s | 2.20 s |
| Native RSS after workload | 44,808 KiB | 44,084 KiB |
| Summed browser process RSS | 846,820 KiB | 855,020 KiB |
| WebSocket received bytes / frames | 37,909 / 77 | 26,712 / 80 |

The p95 increased 7.8 ms and sampled combined CPU increased about 14%; this is
additional presentation cost. No keys were lost. Three-second idle samples
consumed 0.05 → 0.06 s native CPU and 0.10 → 0.11 s browser CPU; these short
samples are variable rather than stable idle baselines. Summed browser RSS
includes shared pages and does not establish retained allocation growth. Video,
hardware acceleration and the unresolved compositor issue are not measured here.

Actual Brave captures at 1200×850 and 480×480 were reviewed for grey Events focus,
colored graph/Mindset/entity focus, outline, reset, fixed-height tuning and compact
component colors. The affected Chromium/Firefox acceptance cases cover focus, geometry and navigation.
Compact colors and the overlap-boundary guard passed three consecutive runs in
both engines (twelve case executions). The
compact test waits for a complete backend frame matching each browser's fitted
terminal dimensions before recording the divider position. Initial test failures
exposed a resize synchronization issue and different browser font metrics. Earlier
full-suite measurements and acceptance results apply to their recorded revisions.

The reported intermittent mountain-over-card overlap has not been reproduced or
claimed fixed. An additional actual activity-demo probe reviewed 125 rendered
frames during repeated tuning, zoom, resize, day navigation and Thought expansion;
no horizontal mountain glyphs crossed into Event-card columns. The browser guard
checks this boundary during repaint as well as in settled views. Synthetic captures
and probe output are under `/tmp/thoughts-daynav-evidence`. These bounded checks
are evidence, not a guarantee about every terminal or compositor.

Final verification included `make ci` (unit, migrated SQLite integration, browser
acceptance and race checks). A subsequent one-path correction makes Q from the
entity picker use the shared focus update; its palette regression failed in both
browsers before the correction and all ten affected browser cases passed after.
Post-correction root race tests and the normal commit/push hooks verify that last
change separately.

## Historical five-row cap and stable loading

At `a7b1a04`, the fullscreen-height follow-up reduced the maximum to five plot
rows, four on medium windows, two on short windows and one on compact windows.
Low boost and maximum fill were the default inside that area; tuning and zoom
did not grow the panel. The fixed graph band and normal baseline supersede
those visual defaults; the measurements below retain their original context.

The graph budget is allocated before Mindset reading space. A regression
reproduced the previous graph shrinking from six to four rows (including its
spacer) when long Mindset content arrived. Data replies and in-range Left/Right
navigation now retain the graph's canvas and cached geometry. The quantitative
measurements above describe earlier revisions, not this five-row correction;
no new performance improvement is claimed.

The compact correction passed focused Go/root/preview tests and race tests, plus
all 22 Chromium/Firefox navigation, clipboard, resize and graph cases. Actual
Brave review covered 1200×850, 1920×1080 and 480×480, including maximum fill,
outline and focus changes. One earlier Firefox run detected a transient buffer
boundary violation during rapid resize; five diagnostic repeats and the final
22-case run passed. The stricter boundary assertion remains, with painted-row
and buffer-row diagnostics. This does not establish the intermittent overlap's
root cause or claim that it is eliminated.

## Calendar-anchored panning

A dramatic one-day-pan regression reproduced a peak changing height/profile
because each new viewport restarted fractional pixel sampling. Vertices now use
an absolute civil-date grid, with the viewport origin aligned to complete Braille
cells. Panning therefore translates the same dated profile instead of changing
its peak rounding or color-cell pairing. The selected-date marker uses this grid.

A separate edge regression reproduced a flattened flank when the last visible
sample became an interior sample. The same bounded read now includes one adjacent
bucket at each edge. These samples continue interpolation outside the visible
range and do not add another database round trip. The existing request limit,
cancellation and stale-result checks remain. Both filled and outline translation
and edge-flank regressions pass at dramatic settings.

The final correction was compared with compact baseline `a7b1a04` on the same
i5-8250U / Linux amd64 / Go 1.25.14 reference machine. Both binaries used the
unchanged 721-Event/20,000-Thought stress fixture, a 40-row terminal and five
one-second runs, sequentially without competing builds/tests. Setup and
compilation were excluded. Medians:

| Width / operation | Before ns/op | After ns/op | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| 80 Load | 6,951,749 | 6,110,406 | 49,349 → 51,994 | 2,436 → 2,457 |
| 80 Rebuild | 123,038 | 125,684 | 14,512 → 15,712 | 86 → 102 |
| 160 Load | 6,170,441 | 6,352,235 | 63,174 → 67,601 | 2,441 → 2,470 |
| 160 Rebuild | 214,921 | 215,114 | 28,416 → 31,232 | 91 → 114 |

These runs show no material time regression, while allocations increase for
neighboring samples, vertex positions and changed style runs. They do not show
statistical significance or an overall performance improvement. The bounded
count request widens by adjacent context rather than adding a database call.

Actual Brave at 1200×850 exercised 45 days left and 31 days right at extra boost
and maximum fill. Across a one-day viewport pan, every shared terminal cell,
including the edge flank, matched an exact five-column translation. Screenshots
were inspected. All 22 browser navigation/resize cases and focused renderer,
day-graph and root race tests passed. Deterministic regressions also cover a high
peak entering from outside the viewport, in filled and outline modes.

## Stable scale and buckets during sustained scrolling

The previous pixel-grid correction (`9376f76`) did not fix viewport normalization.
An actual Brave reproduction changed 230 shared mountain cells after the 33rd
Left press, when a peak moved out of the window. The window also used to regroup
wide-range samples on every one-day pan, changing their measured averages.

The current scale uses the maximum across the full active history at the chosen
bucket resolution. Whole buckets are anchored to civil dates, and sample centers
use those actual dates. The accepted scale and viewport publish together under
the existing owner/request checks. Failed or canceled refreshes retain the old
landscape. Replacing a pending history refresh with zoom retains its refresh
requirement rather than reusing a stale reference. Changing data or resolution may legitimately establish a new scale;
ordinary panning does not. This adds bounded history reads when establishing a
scale, not on each pan. No new store API, schema, dependency, timer, worker or
persistent history cache was introduced.

Focused regressions reproduce both original failures, exercise a remote peak
across three history batches, preserve the scale across 40 pans in each direction,
and cover cancellation, failed refresh, DST and pre-epoch bucket anchoring. Shared
filled/outline cells are checked at dramatic settings. Actual Brave at 1200×850
passed 270 day changes across low/extra/outline modes with zero shared glyph/color
mismatches (90 viewport pans). A separate 45-repeat keydown burst in each direction
reached the exact dates without reconnecting. Representative screenshots were
inspected. One initial manual capture compared a prior style frame against the
next style; the final run explicitly awaited the style change before capture.
This is unrelated to the unresolved intermittent resize-overlap report above.

The same i5-8250U / Linux amd64 / Go 1.25.14 machine compared `9376f76` with this
follow-up using identical benchmarks and unchanged migrated fixtures: stress
721 Events/20,000 Thoughts; two-year activity 3,992 Events/30,493 Thoughts. Each
revision ran separately, without competing builds/tests: five one-second runs,
setup/compilation excluded. `Load` explicitly refreshes the history reference;
`Pan` alternates adjacent viewport positions; `Navigate` stays inside loaded data.
Medians:

| Fixture / operation / width | Before ns/op | After ns/op | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Stress Load/80 | 5,296,940.0 | 19,166,019.0 | 52,023 → 84,189 | 2,457 → 4,773 |
| Stress Rebuild/80 | 116,127.0 | 155,575.0 | 15,712 → 15,712 | 102 → 102 |
| Stress Navigate/80 | 705.1 | 804.1 | 1,408 → 1,408 | 1 → 1 |
| Stress Pan/80 | 5,123,987.0 | 6,069,245.0 | 50,805 → 50,001 | 2,414 → 2,414 |
| Stress Load/160 | 5,623,638.0 | 15,049,426.0 | 67,509 → 99,664 | 2,470 → 4,786 |
| Stress Rebuild/160 | 199,392.0 | 240,742.0 | 31,232 → 31,232 | 114 → 114 |
| Stress Navigate/160 | 1,100.0 | 1,174.0 | 2,688 → 2,688 | 1 → 1 |
| Stress Pan/160 | 5,147,589.0 | 5,461,907.0 | 65,410 → 65,484 | 2,424 → 2,434 |
| Two-year activity Load/160 | 2,945,817.0 | 22,146,728.0 | 49,788 → 341,283 | 1,093 → 14,684 |
| Two-year activity Pan/160 | 2,634,446.0 | 2,812,011.0 | 48,587 → 47,360 | 1,056 → 1,059 |

Cold/explicit-refresh loading now costs roughly 15–22 ms in these fixtures,
versus 3–6 ms before. That is real additional work to establish a stable scale.
Pan medians increased about 0.18–0.95 ms; rebuilds increased roughly 0.04 ms,
and cached navigation remains around a microsecond. This is a correctness tradeoff,
not a speedup. Five runs do not establish statistical significance. No new
whole-app CPU, memory or navigation-p95 improvement is claimed from these tests.

## Normal baseline and consistent browser height at `323c1dd`

Startup and style Reset use normal boost (0.8), 100% size, and filled mode.
This rebases the former low/300% silhouette rather than returning to the older,
smaller gain. Higher and lower settings modulate this baseline inside the fixed
canvas; no setting moves Events downward. The renderer retains the prior fill
calibration so ordinary Braille rounding and outline appearance stay intact.

At that revision, the plot used two rows from a 26-row terminal upward, matching
the accepted small browser window. Very short windows retain their one-row fallback. Actual Brave
review at 480×480, 1200×700, 1200×850, and 1920×1080 verified the same two-row plot
and Events header position. Small-window cells/colors matched the prior binary
exactly. At each larger size, tuning to 300% and resetting restored the exact
normal profile. Screenshots were inspected. Literal preview geometry and root
resize tests preserve these relationships. The earlier performance tables
predate this presentation change; no new timing or CPU improvement is claimed.

## Shared baseline, gap, and status placement

The approved Braille + gap preview uses four mountain rows and one marker row.
The bottom two Braille dots form a continuous baseline in the same cells as the
filled slopes. Outline mode shares that axis without filling its interior. The
accepted date is a single background-colored space in the row below it, matching
the purple Events highlight. Moving that block does not rerasterize mountains
or overwrite any of their dots.

The Events header sits at row 9 with a collapsed Mindset in normal windows
(counting from zero). The five-row graph band cannot grow with zoom, tuning, or
fullscreen height. Short terminals progressively reduce plot rows, retaining the
marker gap and usable Event-card viewport. Status belongs beside the Events date
when both fit, or in the existing footer when they do not. Normal/100%/filled
remains the style default.

The earlier literal two-row profiles and timing tables describe their original
revisions, not this increased plot resolution.

### Focused layout cost

Compared the pre-layout PR head `135f9cd7047a937c6fb88f23d483a0c877812da6`
with the Braille + gap layout at `25bc91e`, using the existing
`BenchmarkDayActivity` Rebuild
and Navigate cases. The same migrated synthetic fixture contained 721 Events and
20,000 Thoughts, all before the accepted cutoff. Binaries were compiled first
and run separately without competing builds/tests. Linux/amd64, Intel i5-8250U
(8 logical CPUs), Go 1.25.14; five 1-second runs per case, medians below.

| Operation / columns | ns/op before → after | Δ time | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Rebuild/80 | 76,330.0 → 142,264.0 | +86.4% | 8,248 → 13,584 | 108 → 85 |
| Navigate/80 | 591.9 → 4,794.0 | +709.9% | 704 → 1,376 | 1 → 8 |
| Rebuild/160 | 128,898.0 → 242,853.0 | +88.4% | 14,704 → 26,816 | 155 → 97 |
| Navigate/160 | 700.1 → 4,582.0 | +554.5% | 1,280 → 2,491 | 1 → 8 |

The final plot rasterizes four rather than two mountain rows. Cached navigation
also composes the solid marker row; it does not rerasterize or query history.
The old Navigate case omitted the root's separate divider composition; the new
component includes its marker row. This is a component-cost comparison, not an
end-to-end navigation regression measurement. These are additional bounded
presentation costs, not speedups. Five runs do not establish statistical significance. This comparison does not measure whole-app
CPU, memory, browser composition, or navigation p95; the earlier query/pan
measurements retain their original revision-specific meaning.

Focused Go/root tests cover the 24–27-row allocation boundaries and loading before
the first reply. All 28 affected Chromium/Firefox cases passed. Brave captures
were reviewed at 1200×850, 380×480, 380×440 and 1920×1080, including normal/300%,
outline, Events focus and expanded Thoughts. Both narrow viewports fitted 43×26
terminal cells; normal/fullscreen used four mountain rows plus the marker gap.
Reset restored the exact normal plot; left/right marker movement, 40 day changes
in each direction and card expansion kept one WebSocket. The manual harness's
initial incomplete resize snapshots were rejected; this is not evidence that the
previous intermittent overlap report has been fixed.

### Loading-feedback follow-up

The first gap-layout revision `25bc91e` displayed loading immediately for each
viewport read. Consecutive pans could keep the label visible while moving even
though individual reads were quick. The follow-up uses the same cancellable
150 ms one-shot feedback pattern as Events. A slow read still reports progress;
newer requests clear the prior label, and an accepted result or failure retires
its timer. Neither data reads nor their application wait for that timer.

Deterministic tests exercise fast/slow reads, stale component/request replies,
cancellation, failures, a selection moving beyond an in-flight pan, and real
root-dispatch feedback/zoom transitions. The browser regression crosses the
month viewport with forty keys each way and checks that feedback settles without
reconnecting. The layout timing table above belongs to `25bc91e`; this follow-up
does not change rasterization or cached marker composition and makes no new
performance claim.

A matched Brave observation on the same synthetic activity fixture used 45 held
Left keydowns and 45 Right keydowns. The `25bc91e` binary showed loading in three
captured render frames (longest observed interval 17 ms); the delayed-feedback
binary showed it in zero. Both returned to the exact date with one WebSocket and
no final loading label. This reproduced brief feedback during movement, not a
permanently stuck label after movement stopped. The lifecycle regressions cover
stale timers and queued pans; this single walkthrough is not a latency or CPU
benchmark.

### Pinned block while panning

The first/last selected dates occupy the outermost columns of the marker gap.
Previously a pan clamped an outside date to the border, then moved the block
inward to its sample center when the new viewport arrived. Pinning edge dates
keeps pending and accepted positions identical, without changing curve samples.
For a one-day viewport, the last movement direction selects the shared edge;
reapplying the accepted date does not reset it. Interior dates retain their
calendar-grid positions.

Deterministic tests check repeated left/right pans at 1, 40, 80 and 160 columns,
including month and one-day zoom. Browser capture checks painted block positions
through forty-five day changes each way, rather than only inspecting a settled
endpoint.

### Now and Mindset day navigation

Now/day-boundary focus survives day arrivals instead of becoming entry focus.
That preserves the latest-card/boundary/metrics composition, including empty
days. Returning to today restores live follow-Now behavior; Right at today does
not open a card. Only entry browsing initializes the first card on a changed day.
Mindset arrows use the same day command while retaining panel focus. Enter is
its sole Thought-opening key; reading and paging stay in the component.

Focused Go tests cover populated/empty day cycling, the first-card distinction,
today's limit, Enter-only Mindset opening and root dispatch. All 36 affected
Chromium/Firefox cases and navigator/Events/Mindset/root race tests passed. The
single changed preview golden line is the Now-view help binding; its geometry
is unchanged.

An actual Brave walkthrough used the varied synthetic diary at 1200×850. Its
Now view kept one latest card plus metrics across dates, Mindset arrows preserved
focus and Enter opened a real synthetic pinned Thought. The temporary pin was
removed through the UI. Reviewed captures and results are under
`/tmp/thoughts-daynav-evidence`; one WebSocket served the whole walkthrough.


## Broad-terrain experiment at `acf5eaf` (height superseded)

The six/eight-row application height tested here was subsequently reverted to
four rows at all zoom levels. The terrain and fixed-canvas measurements remain
valid; taller benchmark canvases do not describe the current application layout.

Synthetic comparison fixtures cover a dominant summit, two separated peaks,
sustained activity, an isolated day, Thought/Event-heavy periods, empty data,
and irregular history. `BenchmarkDayMountainRender` fixes the same 43 daily
samples (31 visible plus context), reference, channels, width, and canvas height
for both versions. `BenchmarkDayMountainCachedNavigation` loads once, then
moves ten dates inside the month without reads. Measurements are five-run
medians, 300 ms per run, with fixture generation and compilation outside timing;
no competing build/test process ran. CPU/hardware metadata is retained in the
benchmark output. These are native render/marker costs, not end-to-end browser
latency or a claim about GPU composition.

| Braille canvas | Linear render, µs | Broad terrain, µs | Go bytes before → after |
| --- | ---: | ---: | ---: |
| 40×2 | 12.1 | 44.1 | 2,432 → 6,816 |
| 80×4 | 30.5 | 103.7 | 5,184 → 11,616 |
| 80×6 | 38.5 | 112.5 | 7,024 → 13,456 |
| 80×8 | 46.9 | 127.2 | 8,864 → 15,296 |
| 120×6 | 55.4 | 176.6 | 10,096 → 18,576 |
| 120×8 | 76.8 | 192.5 | 12,960 → 21,440 |

The terrain mathematics cost roughly 2.5–3.6 times the linear renderer in this
fixture, adding under 0.12 ms per rebuild. Increasing the new 80-column canvas
from four to six rows adds about 9 µs, and six to eight adds about 15 µs. Cached
navigation at a matched four-row canvas remains about 4.0 µs with eight
allocations; normal six-row navigation is about 4.2–4.4 µs. Taller cached strings
increase bytes per navigation from roughly 1.5 to 1.9 KiB. No rendering speedup
is claimed, and ordinary marker movement does not pay the terrain rebuild cost.

Reproduce with `go test ./internal/render ./internal/tui/daynav -run '^$'
-bench '^BenchmarkDayMountain' -benchmem -count=5 -benchtime=300ms`.
The intermittent mountain-over-Event report remains unresolved; passing pure
render/layout tests does not establish a compositor fix. Current browser
qualification and reviewed captures must be reported separately.


Visual qualification for this experiment reviewed actual Go Braille output at
40×2, 80×4/6/8, and 120×6/8 using all seven fixtures. A temporary monotone cubic
shoulder comparison was similarly broad but rounder at the summit; the cosine
shoulder plus small pointed cap was retained. Four rows show the range clearly,
six separate crests and component layers better, and eight add slope detail
without changing the counts. At one-day zoom a daily measurement and its broad
neighboring shoulders can look almost flat: eight rows are not intraday data.

Fourteen focused Chromium/Firefox checks passed, including the per-render
mountain-over-Event guard during six/eight-row zoom transitions, tuning, resize,
and return to an expanded Event. The real two-year activity demo was reviewed at
1200×850 and 480×480, including one-day zoom, panning, outline, and Event expansion.
No mountain-over-card overlap was observed; the historical intermittent issue
is not claimed fixed. Temporary comparisons, captures, benchmark output and
review scripts are under `/tmp/pr92-mountains/`; these are disposable evidence,
not required application files or a permanent preview service.


## Selected B contour

The Spider-Man comparison used identical fresh copies of its 124 Thoughts and
80 Events, at the default month, normal boost, 100% size, and compact height.
Actual app captures showed the former 4–6/3–4-bucket supports merging the seed's
mostly 5–6 Events and 8–9 Thoughts per completed day. B was selected: narrower
1.8–2.7/1.2–1.8-bucket shoulders with equal cosine and linear contributions.
Only contour parameters change; counts, historical scale, four-row cap, compact
layout, colors, and controls stay the same. Earlier wide-terrain benchmark
results above describe their original parameters, not new measurements of B.
