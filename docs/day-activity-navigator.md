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
  chosen toggles persist. The plot uses three grey/white shades while Events has focus, and is colored
  in the day graph, Mindset, and entity picker.
- `d` opens local distribution controls: Left/Right changes boost, Up/Down
  changes size, `f` switches the mountains between filled and outline, `0` resets
  styling. Escape/Enter closes controls. Down or Escape outside controls returns to Events.
- `r` explicitly refreshes activity. These display preferences last for the session.

On Mindset or the entity picker, `d` cycles session-local distribution palettes:
all colored → all neutral → metrics only → mountains + metrics → Events + metrics.
Metrics bars are colored as a group except in all-neutral mode. Neutral mountains
retain their grey/white layers and the one-row Total fallback. Switching panels
preserves the mode. Events and mountain `d` controls still adjust their shapes;
selection and keyboard-focus highlights remain independent.

When focus is outside the Events timeline, the selected palette controls which vertical Event curves are highlighted;
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
    Total level      = (Combined / maximum Combined)^exponent
    Event level      = 0.75 × Total level × Event activity / Combined
    Thought level    = 0.75 × Total level × Thought activity / Combined

Zero combined activity produces zero levels. An Event retains greater per-record
weight, while many Thoughts naturally outweigh fewer Events. All three use the same weighted reference. A shared 0.75 component-height
factor reserves at least a quarter of Total's height above either component;
this is display separation, not a change to the underlying counts. The ratio
between Event and Thought band thicknesses follows the data. Local weighted
shares are blended over the same envelopes; Thoughts sit above visible Events
inside Total, so both components follow its ridge. Hiding Events removes their
offset beneath Thoughts without changing Total.

Zero channels remain zero. The fixed weights give Events greater visual weight;
there is no user-facing weight setting. Boost presets use exponents
1 (low), 0.8 (normal, default), 0.5 (medium), 0.25 (strong), 0.15 (extra).
Higher boost lifts quieter activity without moving the reference summit, matching
the Event distribution controls. Size ranges from 50–300%; startup and Reset use normal boost at 100%.
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
foreground color, so filled color boundaries remain stepped; no dots are removed
to smooth them. Thin bands can still share cells at compact sizes. When only one mountain row
fits and Total is enabled, show its grey contour alone rather than giving the
combined mountain an Event or Thought color. Hiding Total reveals the selected
components; a taller viewport restores their layers without changing the toggles. Unfocused
Total, Thoughts and Events use terminal shades 240, 244 and 248 respectively.
Outline mode retains only the owning contour’s dots in each shared cell, with
blank interiors and no separate baseline. Hiding a channel does not renormalize
Total or the remaining channel’s measured share.


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
keeps its full-text reading and scrolling behavior. Loading feedback uses one cancellable 150 ms timer per read; there is no animation timer, new WebSocket protocol or browser-only drawing.

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

## Performance evidence

Measurements are revision-specific, not current guarantees. Native microbenchmarks do not measure browser CPU, key-to-display latency or total application responsiveness. Run revisions separately with identical fixtures, clocks, dimensions and toolchains; exclude migration, seeding and compilation. Keep the original stress seed unchanged.

### Original timeline comparison

On 2026-10-23, i5-8250U, Linux amd64, Go 1.25.14: five one-second runs compared merged PR #88 (`99d791b`, built from `97b1bf0` plus the bounded-query fix) with the initial day navigator before its Mindset rebase. This measured the daily renderer, not the whole root view or plot.

| Size / operation | Before ns/op | After ns/op | B/op before → after | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 80×27 Render | 898,003 | 903,498 | 93,315 → 93,409 | 2,321 |
| 80×27 Navigate | 1,378,846 | 1,365,722 | 156,892 → 156,718 | 3,812 |
| 80×27 Expanded | 905,822 | 930,268 | 131,704 → 132,082 | 3,143 |
| 40×20 Render | 533,630 | 555,969 | 80,747 → 80,634 | 2,108 |
| 40×20 Navigate | 975,108 | 968,815 | 143,244 → 143,588 | 3,483 |
| 40×20 Expanded | 683,300 | 697,952 | 91,206 → 90,973 | 2,465 |

The original headful Brave stress comparison at 1200×850 measured median key-to-display 42.3 → 41.45 ms and p95 47.3 → 54.0 ms. Sampled combined CPU rose about 11%. This was added presentation work, not a speedup; five groups of twelve keys are limited evidence.

### Stable historical scale tradeoff

The stable-scale follow-up compared `9376f76` with its replacement on the same i5-8250U/Linux/Go toolchain. Five one-second runs used unchanged migrated fixtures: stress 721 Events/20,000 Thoughts; two-year activity 3,992 Events/30,493 Thoughts. Load refreshes history; Pan reads adjacent viewports; Navigate remains inside loaded data.

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

### Formatting cleanup against `f74f8e1`

Measured 2026-10-28 on the same i5-8250U, Linux amd64, Go 1.25.14. Fresh databases were migrated with go-migrate and seeded from the unchanged stress (721 Events/20,000 Thoughts) and two-year activity (3,992/30,493) scripts. Revisions ran separately, without competing builds/tests; compilation and setup were excluded.

Five 300 ms runs on the stress fixture, medians:

| Operation, 80 columns | Before | After | Bytes before → after | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Styled redraw | 282.1 µs | 96.0 µs | 25,760 → 15,200 | 415 → 22 |
| Cached navigation + View + marker | 4.57 µs | 4.47 µs | 2,144 → 2,144 | 8 → 8 |

Three 200 ms runs also measured stress redraw at 160 columns: 426.4 → 191.6 µs, 44,088 → 26,208 bytes, 462 → 22 allocations. Two-year activity redraw at 80 columns: 201.5 → 85.8 µs, 20,096 → 13,152 bytes, 238 → 21 allocations; at 160 columns: 298.0 → 175.2 µs, 33,016 → 22,112 bytes, 253 → 21 allocations. These short comparisons are not statistical significance claims.

The measured saving removes full Lip Gloss layout from foreground-only runs and intermediate run buffers. It does not change terrain mathematics, historical scaling or database reads. In a separate three-second stress redraw profile, Lip Gloss Render accounted for about 62% cumulative CPU before this change. No-op tuning now allocates zero bytes and retains its plot. Five 200 ms presentation runs cover filled/outline rebuilds, palette changes and tuning at a bound.

The prior standalone 80×4 terrain benchmark measured 70.5 µs (five 300 ms runs). Initial/refresh loads were 11–24 ms across the two fixtures; pans were 3–6 ms. They remain database work, not the formatting optimization's claimed benefit. No whole-app CPU, memory or browser-latency improvement is inferred.

Reproduce using disposable migrated fixtures:

```sh
THOUGHTS_STRESS_DSN=file:/path/to/stress.db go test ./internal/tui/daynav \
  -run '^$' -bench '^BenchmarkDayActivity/(Rebuild|Navigate)/80$' \
  -benchmem -count=5 -benchtime=300ms
go test ./internal/tui/daynav -run '^$' \
  -bench '^BenchmarkDayMountainPresentation$' -benchmem -count=5 -benchtime=200ms
```

A temporary comparison checked byte-identical plot output against the previous formatter across 36 width/height/palette/fill combinations. Edge/pan tests now use the same explicit calendar sample context as production, removing an unused alternate input contract. Duplicate fixed-height coverage was consolidated; ownership and layout regressions remain.

## Remaining limitations

- Daily buckets cannot reveal intraday peaks. The terrain is a bounded visual interpretation, not a new measurement.
- A terminal cell has only one foreground color; thin component bands can disappear inside a shared cell. One-row Total intentionally remains grey.
- Initial and explicit-refresh history work grows with retained history. Panning reuses its scale, and each query remains bounded to 256 buckets.
- Prior browser runs were variable: one default-view comparison increased p95 by 7.8 ms and sampled combined CPU by about 14%. Native cleanup results must not be presented as browser improvements.
- The stale-marker grow scenario was reproduced in the actual dense diary. Fixed-size browser replay also reproduces stale mountain cells from the old Go renderer: its widened output and cache disagreed. The pinned upstream correction resizes the cache before drawing. Rapid resize also exposed stale scrolling margins from an old-size frame; the browser output boundary now resets margins before a native full repaint. These are separate from the retained xterm right-margin correction; these bounded regressions do not establish a universal compositor fix. See [the exact reproductions and ownership](../internal/browserterm/UPSTREAM.md#native-renderer-resize-cache).

Superseded height/contour experiments are omitted here. Current controls and geometry above are authoritative; earlier measurements describe their original revisions.
