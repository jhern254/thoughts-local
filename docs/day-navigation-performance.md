# Historical day navigation: bounded Event coverage

Measured 2026-10-23 on an Intel i5-8250U (8 logical CPUs), Linux amd64,
Go 1.25.14, modernc SQLite 3.51.2, and headful Brave 148.0.7778.217.
Before: master `97b1bf0c17b67672fa51841c354e2e776b996b0b`.
After: this PR's bounded `covering_events` query; no rendering or input-routing
production changes. Binaries were compiled before timing. Runs were sequential,
without competing builds/tests. Five repetitions are descriptive evidence, not
proof of statistical significance.

## Cause and change

`ListEntriesInRange` reads Events and then finds Thoughts outside Event intervals.
Previously the latter query searched all preceding Events separately for each
Thought in the requested day. The stress fixture makes that expensive even when
all Thoughts are covered and the query returns no groups.

The replacement materializes only Events overlapping the requested window once.
The per-Thought half-open coverage check then examines those intervals. Spanning,
ongoing, overlapping and zero-duration Events retain their existing semantics.
There is no new index, schema, database round trip, application cache, input
throttling, or video-quality change.

Actual SQLite 3.51.2 query-plan differences:

```text
Before: CORRELATED SCALAR SUBQUERY
          SEARCH e USING INDEX idx_events_user_started_at
            (user_id=? AND started_at<?)
After:  CORRELATED SCALAR SUBQUERY
          MATERIALIZE covering_events
            SEARCH events USING INDEX idx_events_user_started_at
              (user_id=? AND started_at<?)
          SCAN e
```

Both retain the active chronological Thought index. Old Event history is still
examined once to find overlaps, rather than once per Thought. A heavily overlapping
single day can still require more work; this does not promise constant-cost reads.

## Native measurements

The unchanged `scripts/demo_stress.sql` fixture has 721 Events and 20,000 Thoughts.
Its fixed anchor is Unix `1792739869`; displayed days use America/Los_Angeles.
Both revisions used the same disposable migrated database. Historical reads
alternate between the anchor's third and second preceding days. Batch includes
List, Counts and SubjectTime, sharing the application's one-connection pool.
Compilation, migration and seeding are excluded.

Medians of five 1-second runs:

| Operation | Before ns/op | After ns/op | Time change | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Historical timeline list | 142,324,677 | 2,073,995 | −98.5% | 25,276 → 24,626 | 546 → 548 |
| Historical concurrent batch | 136,643,324 | 3,983,148 | −97.1% | 30,172 → 29,963 | 697 → 697 |
| Historical uncovered-Thought query | 123,267,958 | 1,492,384 | −98.8% | 1,104 → 1,299 | 26 → 31 |
| Current-day Event list control | 417,228 | 390,610 | −6.4% | 1,980 → 1,979 | 51 → 51 |
| Current-day count control | 633,913 | 648,077 | +2.2% | 1,134 → 1,141 | 29 → 29 |
| Subject-time control | 436,517 | 400,585 | −8.2% | 1,765 → 1,769 | 52 → 52 |

Controls are unchanged production code; their differences illustrate run variance.
The optimized query adds five small allocations while removing repeated scans.
Resident renderer smoke checks (3 × 200 ms) remained around 0.9 ms at 80×27 and
0.6 ms at 40×20; they do not measure database navigation.

```sh
TZ=UTC THOUGHTS_STRESS_DSN=file:/tmp/stress.db go test -tags=integration \
  ./internal/tui/events -run '^$' \
  -bench 'BenchmarkTimelineDaySwitch/Historical|BenchmarkTimelineSQLite/(List|HistoricalThoughtGroups|Counts)$|BenchmarkSubjectTimeInRange$' \
  -benchmem -count=5 -benchtime=1s
```

## Browser measurements

The focused `THOUGHTS_PERF_DAY_ONLY=1` mode extends the existing background probe.
It starts three days in history and verifies the expected date **and populated
cards**, rather than accepting an unrelated terminal mutation. Each of five runs
per condition contains 20 individual keys and three bursts: eight Left keys,
eight repeated Right keydowns, and alternating keys ending three days earlier.
The accepted date must remain correct after obsolete replies can arrive.

Static mode uses the exact prepared video's poster. Video is a synthetic 5-second
1920×1080 H.264 High-quality-tier clip at 30 FPS, playing at 100%. Prepared SHA-256:
`a08d976a57eef68baf9ca873c9df796283ec95a67d1f48f2b5243539b19b564e`.
Both measured revisions produced the same prepared bytes. Import is outside
navigation timing. Each condition uses a fresh page; mode order rotates by run.

1200×850 viewport; 100 single-key samples per condition. Values are keydown to
xterm render of the expected destination, not proof of GPU presentation:

| Background | Median ms before → after | p95 ms before → after | p99 ms before → after |
| --- | ---: | ---: | ---: |
| Static poster | 159.6 → 35.9 | 168.6 → 48.4 | 175.7 → 55.3 |
| Paused video | 158.6 → 32.4 | 171.7 → 58.2 | 182.8 → 62.7 |
| Playing High video | 159.9 → 42.9 | 178.7 → 47.8 | 208.6 → 50.4 |

No timeout or incorrect final date occurred in either revision: 300 individual
transitions and 45 bursts per revision. Median last-key-to-final-render burst
latency with playing video fell from 157.2 to 40.7 ms. Lost input was not reproduced;
the demonstrated issue was delayed feedback. Root-dispatch tests additionally
exercise repeated keys and successful stale replies delivered after a newer day.

Median CPU milliseconds for the same 49-key workload (including driver waits):

| Background | Native CPU before → after | Browser CPU before → after |
| --- | ---: | ---: |
| Static | 3,960 → 1,150 | 620 → 750 |
| Paused | 4,010 → 1,150 | 660 → 800 |
| Playing | 4,210 → 1,260 | 1,300 → 1,170 |

This measures work per traversal, not idle or steady playback CPU. Static/paused
browser CPU increased modestly while native and combined CPU fell substantially.
Browser RSS is inconclusive: medians were approximately 813–828 MiB before,
901–917 MiB on an initial after run, and 1,011–1,024 MiB on a repeated after run
with a fresh app and no preselected background. Native RSS was about 36 MiB
before and 40–42 MiB after. These short, differently paced workloads do not
control garbage-collection timing; no memory improvement or equivalence is
claimed. The repeated after run is the latency/CPU table above; the earlier run
also had a playing-video p95 of 50.4 ms. Neither after run dropped video frames
during the measured traversals. This does not establish hardware decoding.

A separate 650×600 walkthrough covered the same three backgrounds and burst
patterns (one run each), with correct final dates and focus. Normal/narrow
captures were inspected. The previously reported dark wallpaper rectangles
remain visible; no compositor fix is claimed.

To reproduce against a **disposable** running stress demo, using an independently
generated synthetic video and the app's PID:

```sh
cd internal/browserterm/client
THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 \
THOUGHTS_PERF_INTERACTIONS=1 THOUGHTS_PERF_DAY_ONLY=1 \
THOUGHTS_PERF_VIDEO_QUALITY=high THOUGHTS_PERF_SAMPLES=5 \
THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/synthetic.mp4 \
node background-performance.mjs http://127.0.0.1:7893 APP_PID /tmp/day-results.json
```

Use `THOUGHTS_PERF_DISMISS_SPEECH_SETUP=1` for an app built without native speech
that shows the initial “Not now” prompt; this only dismisses that test setup UI.
Use `THOUGHTS_PERF_INTERACTION_NARROW_ONLY=1` for 650×600. Start before/after
comparisons with no selected background. Raw logs and synthetic captures from
this investigation are under `/tmp/thoughts-day-evidence/` (not permanent assets).
Keyboard acceleration remains a separate UX decision.
