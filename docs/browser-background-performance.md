# Static background budget and performance

Imports retain the existing source limits: non-animated JPEG/PNG, 16 MiB,
8192 pixels per axis, and 24 million decoded pixels. Stored imports are reduced
proportionally to at most 4 million pixels and 3072 pixels per axis, without
upscaling. All new imports re-encode to remove private metadata and normalize
orientation into pixels. JPEG uses quality 85; PNG retains transparency. Existing
assets are unchanged; reimporting applies the budget and sanitization policy.

The durable product policy is documented in [Media handling and privacy](media-handling-policy.md). This document records implementation details, measurements, and limitations for specific revisions; historical benchmark results are not current privacy guarantees. For current video limits and the 1080p qualification, see [Video quality through 1080p60](#video-quality-through-1080p60); earlier video sections retain the limits of their measured revisions.

This bounds the asset used during normal operation, not source decoding during
preview and native validation. Resizing spends CPU once to reduce subsequent
activation work. Darkness/framing changes reuse the decoded active image.

## Reproduction

Run each revision separately on migrated disposable stress demos, with no
competing builds/tests. The probe imports and removes backgrounds; never use
personal data. Install the existing client test dependencies with `npm ci`, then
run from the repository root:

```sh
THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 \
THOUGHTS_PERF_REVISION=$(git rev-parse HEAD) \
node internal/browserterm/client/background-performance.mjs \
  http://127.0.0.1:PORT APP_PID /tmp/background-results.json

go test ./internal/visual -run '^$' -bench '^BenchmarkPrepareImage$' \
  -benchmem -count=5 -benchtime=1s
```

The default is headed Brave (`/usr/bin/brave-browser`); override with
`THOUGHTS_PERF_BROWSER`. Actual fullscreen viewport/GPU metadata and fixture
hashes are recorded. PNG generation occurs before browser measurement. Native
fixtures are constructed outside the timer; the baseline gets only a benchmark
fixture calling its existing validator, not production changes.

The default probe uses five repetitions and 30-second idle samples. This final
comparison retained five import/interaction repetitions but used one 30-second
candidate idle sample per mode (`THOUGHTS_PERF_IDLE_SAMPLES=1`) at the owner’s
request to shorten verification. Baseline idle medians still use five samples.
Candidate idle values are descriptive snapshots, not equally sampled medians. CPU is a
percentage of one logical CPU, with browser processes and Go measured separately.
Summed browser RSS double-counts shared pages; PSS is also recorded. Peak RSS
sampling at 100 ms can miss shorter peaks. Automation and contention workers
are excluded from process totals. Navigation measures keydown to a terminal DOM
mutation, not the physical display pipeline. Each picker repetition contains
20 selections. Resize latency starts at the browser resize event; pair duration
includes intentional settling. Contention uses four workers on this eight-thread
machine, not an ML workload simulation. Five runs do not establish statistical
significance.

Raw recovered results are kept locally in ignored `bin/background-evidence`;
synthetic visual captures remain under `/tmp/background-reliability-profile`.

## Reliability

The reported upload → Apply → entity picker → immediate Left/Right sequence is
exercised with 91 × 4093 portrait, 4093 × 91 panorama, and 6000 × 4000 images.
Reviewed Brave captures at 1920 × 1080 and 948 × 983 after portrait upload and
immediate entity selection did not reproduce the intermittent black blocks;
this change does not claim a compositor fix. Browser tests cover framing,
orientation, failed replacement, reload, removal, focus, clipboard, drafts, and
image import during recording. SQLite tests close and reopen the runtime to
verify optimized background persistence. Controlled async tests reject late
metadata/decode results and retire candidate URLs safely.

No schema changes, video playback, recurring background work, terminal transport
changes, or voice architecture changes are introduced.

## Results

Intel Core i5-8250U, eight logical CPUs, Linux amd64, Go 1.25.14, Brave
148.0.7778.217. Baseline: `f34d0b956fe188aad44dc262dd18b683f963e9e0`.
Production candidate: `51df1139122b920a310304adf32ceef2fc713e63`; subsequent
changes add acceptance evidence only. Both databases contain 721 Events and
20,000 Thoughts. Fullscreen is 1920 × 1080 at DPR 1; smaller-window resize is
948 × 983. Source hashes and actual viewports match across every import.

Native fixture medians, five runs:

| Source PNG | Before ns/op | After ns/op | Before B/op | After B/op | Before/after allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1267 × 719 | 12,867,055 | 12,793,942 | 3,709,023 | 3,709,021 | 22 / 22 |
| 6000 × 4000 | 329,682,782 | 602,856,538 | 96,094,304 | 114,645,344 | 22 / 62 |

The large native import is **83% slower** and allocates **19% more bytes**.
This material one-time resize/encode cost moves work to import, not navigation.
The RGBA scaler destination avoids per-pixel interface allocations encountered
in the initial prototype. These are the original pre-sanitization measurements; the privacy follow-up
below supersedes the small-image original-byte optimization.

Browser import medians (ms), five runs. Preview includes opening/closing the
native file picker and waiting for the TUI preview; Apply includes control
navigation, upload, persistence, activation, and return to the picker.

| Source / bytes | Stored pixels | Preview before/after | Apply before/after | Activation before/after |
| --- | --- | ---: | ---: | ---: |
| 1267x719 / 55,183 | 1267 × 719 | 128.4 / 863.9 | 287.0 / 372.5 | 21.9 / 27.8 |
| 91x4093 / 22,178 | 68 × 3072 | 129.9 / 134.9 | 247.5 / 317.0 | 11.6 / 10.1 |
| 4093x91 / 43,970 | 3072 × 68 | 129.9 / 131.2 | 247.2 / 315.9 | 11.7 / 24.7 |
| 6000x4000 / 638,448 | 2449 × 1632 | 639.6 / 1668.6 | 958.7 / 1009.5 | 122.2 / 34.6 |

Large activation is **72% faster**. Its browser CPU for the whole import falls
1510 → 1160 ms, while native CPU increases 310 → 540 ms. Peak summed browser
RSS falls 1409.6 → 1178.6 MiB (16%); peak native RSS increases
250.6 → 268.0 MiB (7%). These peaks do not establish a settled memory saving.

Preview and small Apply wall times regress in this run and must not be hidden.
Preview timing is highly variable (roughly 0.13–1.87 seconds for normal images)
and includes file-picker/automation handoff. Normal preview processing dimensions
are unchanged; large thumbnail dimensions remain 1200 × 800 on both revisions.
Lower large-import browser CPU despite higher preview wall time suggests waiting
rather than added image-processing CPU, but this is an inference, not an isolated
profile. No further optimization or broad performance claim is justified here.

Interaction ms: median of five per-run medians; p95 columns are the median of
five per-run p95 values, not pooled percentiles.

| Interaction | Median before/after | p95 before/after |
| --- | ---: | ---: |
| Picker, 1267x719 | 29.9 / 28.1 | 35.4 / 36.0 |
| Picker, 91x4093 | 26.6 / 28.1 | 32.7 / 32.9 |
| Picker, 4093x91 | 26.8 / 17.2 | 32.3 / 33.2 |
| Picker, 6000x4000 | 20.4 / 23.9 | 30.8 / 32.7 |
| Picker, four contention workers | 28.3 / 26.2 | 43.2 / 32.8 |
| Events scroll | 33.3 / 18.3 | — |
| Day navigation | 31.4 / 28.4 | 39.0 / 38.7 |
| Draft typing | 34.6 / 21.1 | 39.6 / 24.7 |
| Resize to smaller window / fullscreen | 7.6 / 9.1; 5.2 / 3.6 | — |

No picker navigation timeouts. Large-image picker median rises 17% (3.5 ms),
while its p95 remains about 33 ms; no material interaction latency problem was
observed. Apparent improvements in unrelated typing/scrolling are not claimed
as effects of this refactor.

Idle: five-sample baseline median / single candidate 30-second snapshot.

| Mode | Browser CPU % | Native CPU % | Browser PSS MiB | Native RSS MiB |
| --- | ---: | ---: | ---: | ---: |
| opaque | 0.83 / 1.20 | 2.06 / 1.93 | 353.69 / 362.25 | 36.81 / 42.09 |
| 1267x719 | 0.80 / 1.13 | 2.09 / 2.12 | 381.87 / 391.68 | 38.58 / 42.36 |
| 6000x4000 | 0.90 / 1.33 | 2.06 / 2.06 | 394.06 / 391.92 | 40.51 / 44.71 |

CPU remains low in these snapshots, but reduced candidate sampling cannot prove
equivalent idle cost. This establishes a repeatable static-background baseline
for a later video PR; it does not promise lag-free operation with arbitrary ML
workloads. The repeated source limits still permit significant transient memory
on import, even though stored assets are bounded.

## Import privacy and color contract

Every new import, including small images, now decodes and re-encodes. The native
pipeline reads only EXIF orientation, applies all eight rotation/mirror variants
to pixels, then resizes. Stored dimensions match the browser's display-oriented
`createImageBitmap` preview; no EXIF/eXIf is needed afterwards. Re-encoding drops
GPS, dates, device identifiers, thumbnails, XMP, IPTC, comments and PNG text or
other ancillary metadata. Existing files are not rewritten at startup;
replacement/reimport applies these rules to the new copy.

Go's JPEG/PNG codecs do not perform ICC color management. The scaler interpolates
encoded components without converting color spaces. Therefore RGB or grayscale
ICC matrix/TRC profiles accompany the same encoded components and matching
output color model. Their numeric matrix, transfer curves, white/black point,
chromatic adaptation and luminance tags are retained; private/descriptive tags,
source header identifiers and timestamps are removed. Required profile text is
replaced with fixed application text and v4 profile IDs are recalculated.
JPEG multi-segment profiles are assembled by sequence number and written back in
order. PNG stores a sanitized iCCP profile with a fixed keyword. Otherwise its
sRGB declaration, or gAMA/cHRM pair, is preserved. Conflicting authoritative
color declarations are rejected. Profiles are bounded to 4 MiB decompressed and
4096 tag records independently of the source-image limits.

This does **not** implement an ICC conversion engine. CMYK JPEG, LUT-based or
non-matrix ICC profiles, incompatible gray/RGB profiles (including profiled
gray-plus-alpha PNG), and PNG HDR declarations are safely rejected rather than
converted incorrectly or stripped. Untagged images retain browser-default color
interpretation. JPEG remains lossy at quality 85. PNG retains alpha and 16-bit
sample depth. Resizing remains interpolation in encoded component space, not a
new linear-light resampler. Bounded decoder/scaler/encoder calls are not individually
preemptible; request cancellation is checked around them, with no detached work.

Browser acceptance uses a synthetic wide-gamut matrix/TRC profile. It first
establishes that tagging changes browser-rendered pixels, then compares source
and sanitized output after browser color conversion: PNG samples must match
exactly, JPEG samples within four channel levels. It also checks all eight
orientations, crop dimensions, metadata removal, and immediate entity navigation.
No external profile bytes or additional runtime dependencies are introduced.

## Privacy follow-up measurements

Compared the clean pre-cleanup PR head `ca088c91e153dcdf80fb8de70172bf8c2444db3d`
against the sanitization implementation, on the same i5-8250U / Go 1.25.14 / Brave
148.0.7778.217 environment. Native benchmark fixtures remain identical to the
original comparison. Each revision ran separately, with no competing builds or
tests. Five-run medians are descriptive, not a significance claim.

| Native PNG import | Before ns/op | Sanitized ns/op | Before B/op | Sanitized B/op | Before/after allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1267 × 719 | 13,009,053 | 40,917,107 | 3,709,022 | 4,998,588 | 22 / 61 |
| 6000 × 4000 | 602,029,619 | 604,821,536 | 114,645,344 | 114,650,624 | 62 / 67 |

Small imports now cost about **28 ms more (+215%)** and allocate about **35% more**
bytes because privacy requires encoding rather than preserving original bytes.
Large native cost is essentially unchanged in this sample (+0.5%); it already
performed resizing and encoding. Orientation-bearing images additionally perform
a bounded pixel permutation; these timing fixtures have orientation 1.

The existing browser probe's `THOUGHTS_PERF_IMPORT_ONLY=1` mode measures five
imports each of 1200 × 850 (57,646 bytes) and 6000 × 4000 (638,448 bytes), including
Apply → return → entity Left/Right, without repeating the earlier idle/navigation
matrix. Source hashes match across revisions; both migrated stress DBs still
contain 721 Events and 20,000 Thoughts. Actual viewport is 1920 × 1080, DPR 1.

| Browser metric, median | Normal before/sanitized | Large before/sanitized |
| --- | ---: | ---: |
| Preview including native picker, ms | 865.9 / 250.0 | 1671.9 / 1570.4 |
| Apply, ms | 345.4 / 351.9 | 986.0 / 960.8 |
| Apply through return and entity selection, ms | 461.9 / 489.4 | 1141.8 / 1123.8 |
| Activation, ms | 24.9 / 12.4 | 32.9 / 28.6 |
| Browser CPU during import, ms | 660 / 640 | 1270 / 1180 |
| Native CPU during import, ms | 210 / 220 | 560 / 550 |
| Peak summed browser RSS, MiB | 914.9 / 911.8 | 1197.9 / 1206.5 |
| Peak native RSS, MiB | 43.0 / 46.1 | 265.9 / 267.7 |

Normal Apply through return rises about 27 ms (+6%); peak native RSS rises about
3 MiB. The small-image encoding cost is one-time. These runs show no material
increase in large import/activation cost or recurring work, and do not prove
identical performance. Preview wall time remains variable and includes desktop
picker handoff. Two candidate attempts timed out at that handoff before a large
import; their logs are retained separately. The completed comparison uses five
successful imports per image, with a 15-second picker/preview timeout. It does
not attribute the handoff failures to native processing or claim a preview speedup.
Raw measurements are in ignored `bin/background-evidence/privacy-*`; captures
remain under `/tmp`. Settings-only reuse and generation ownership remain covered
by the existing controlled browser tests.

## Looping video: bounds and measured costs

The first video implementation uses self-contained SDR H.264 MP4/MOV inputs,
128 MiB, 1–30 seconds, 4096 pixels per axis, 9 million pixels per frame and 60
FPS. Aggregate decoded work is capped at four billion pixels. Processing/staging
has a two-minute deadline, one decoder/encoder/filter thread, a 64 MiB
single-allocation limit and 64 KiB probe/diagnostic budgets. These are resource
restrictions, **not an OS sandbox or a hard total RSS ceiling**. Input references
are checked before the restricted MOV demuxer opens the private staged file.

Prepared assets use Constrained Baseline H.264, Level 3.2, CRF 23, `veryfast`,
3 Mbps VBV ceiling/6 Mb buffer, 8-bit `yuv420p`, even square-pixel dimensions,
at most 1280 pixels per axis/921,600 pixels per frame, 24 FPS and 16 MiB. Level
3.2 accommodates macroblock padding for non-16-aligned shapes within this pixel
budget. Output is independently probed, checked for fast-start layout and fully
decoded. Audio, other streams, chapters, source metadata and SEI payloads are
removed. SDR Rec.709 is supported; untagged SDR is interpreted as Rec.709.
Explicit incompatible declarations, ICC video profiles and arbitrary display
transforms are rejected. Cardinal rotations are normalized into frames.

A poster is generated once from the validated prepared file. Explicit Rec.709
matrix/range decoding produces bounded RGB bytes; Go encodes the opaque JPEG at
quality 85. Directly encoding the video's YCbCr into JPEG produced a confirmed
matrix mismatch: the uniform-band browser regression differed by 14 channel
levels before correction and passes the eight-level JPEG tolerance afterward.
Tiny moving edges are unsuitable for this test because JPEG chroma loss is
independent of a matrix error. The synthetic uniform-band fixture contains no
personal media. The poster contains no source metadata.

### Codec spike

The unchanged stress app at `3dc2a916bc4bc0e13b55f1f67d9042eba7e04b82`
received a supported JPEG still; a temporary browser-only substitution compared
synthetic eight-second 1280×720/24 FPS Baseline and Main clips (Level 3.1).
Three separate 20-second normal-speed samples on the i5-8250U/8 logical CPUs,
Brave 148.0.7778.217, Linux and Go 1.25.14 produced median browser CPU of
21.48% and 22.53% of one logical CPU respectively, with zero dropped frames.
Baseline remains the selected profile; this is not a statistically significant
speed comparison or proof that Main is universally slower. Firefox and Chromium
subsequently executed the actual prepared Baseline/Level 3.2 assets, including
poster-color, playback and lifecycle acceptance.

A separate compatibility check played both identical spike clips past two
seconds in Chromium 149.0.7827.55 and Firefox 151.0 at 1280×720, with zero
reported dropped frames in those short samples. This verifies basic playback
of both profiles, not their relative CPU cost or hardware decoding. Its results
are retained in `/tmp/thoughts-video-spike/profile-compatibility.json`.

Baseline fixture: 3,114,995 bytes,
SHA-256 `14a75c20ac292d05a13264f2a3312265bd3334c92b9c734b0558e5a6bf39686f`.
Main fixture: 2,446,590 bytes,
SHA-256 `85ed1f590b0987afaecf83454bb54ff8e79f5f1e5af51f054519b1af1b89f408`.
Both use `testsrc2`, CRF 23/veryfast, 3 Mbps maxrate/6 Mbps buffer, one thread and
Rec.709 signaling. Fixture generation finishes before measurements.

### One-time native import

Five `BenchmarkPrepareVideo` runs used the identical pre-generated Baseline
fixture, `-benchtime=1x -benchmem`, with compilation excluded and no competing
builds/tests. This includes staging, probing, encoding, complete output decoding,
poster preparation and cleanup; it excludes selection persistence.

| Revision | Median ns/op | B/op | allocs/op | Maximum process/child RSS |
| --- | ---: | ---: | ---: | ---: |
| `401df012c8893927de1c23bd15a70664ffd30844` | 2,415,442,130 | 41,426,904 | 922,243 | 110.4 MiB |
| Opaque RGBA correction, `1b09424` | 2,383,168,794 | 37,736,400 | 640 | 110.6 MiB |

Opaque RGBA uses the standard JPEG encoder's direct pixel path. The measured
allocation reduction is 99.93% and bytes reduction 8.91%; the 1.34% elapsed
difference is not a significance claim. FFmpeg dominates one-time cost. GNU
`time` reports the largest process/child RSS, not simultaneous summed RSS; Go
allocation counters exclude FFmpeg. User/system CPU across all five imports was
11.35/0.79 seconds before and 11.26/0.81 seconds after. Later upload/playback
ownership corrections do not change this RGB/encoding path.

### Recurring playback and interaction

Actual implementation `7d720a2` used the same clip and the migrated stress
fixture (721 Events/20,000 Thoughts) on the same reference machine. The existing
probe was extended for video; a temporary owned host used the real application
services and browser PCM transport with a discard consumer. It measures capture
and ingestion, **not speech inference or another ML pipeline**. Each steady
state below has one 30-second sample after settling. CPU is relative to one
logical CPU. Browser PSS sums the measured browser processes; RSS double-counts
shared pages. Native CPU/memory is recorded separately.

| State | Browser CPU | Native CPU | Browser PSS |
| --- | ---: | ---: | ---: |
| Static image | 1.20% | 2.13% | 312.6 MiB |
| Video 100% | 23.25% | 2.23% | 388.7 MiB |
| Video + synthetic recording | 39.68% | 3.06% | 413.1 MiB |
| Video 50% | 15.23% | 2.29% | 413.1 MiB |
| Video 25% | 7.81% | 2.36% | 413.4 MiB |
| Paused | 0.83% | 1.96% | 408.2 MiB |
| Reduced motion | 0.93% | 1.96% | 408.7 MiB |
| Hidden visibility emulation | 0.93% | 1.96% | 409.1 MiB |

Brave's actual player properties reported `VaapiVideoDecoder` and
`kIsPlatformVideoDecoder=true`; codec capability hints alone were not used as
hardware proof. **The additional normal-speed CPU target below 10% was missed**
(+22.05 points). Native recurring cost changed little. This synthetic moving
pattern is busy; quieter content/hardware can differ, but acceptable cost is not
guaranteed. Pausing, using a smaller/lower-FPS source or retaining an image are
simple available tradeoffs. No cache, frame-copy loop or recurring workaround was
added. Quarter speed presents roughly six frames per second and can look choppy;
50–100% is the simple recommendation for smoother motion. No interpolation is
implemented.

Five 20-key Events navigation groups gave median group p95 of 33.8 ms with the
static image and 40.4 ms with video (+6.6 ms), below the 10 ms target in this
run. No statistical significance or universal latency guarantee is claimed.
Five video selections had median preview 1942 ms, Apply-to-visible-player 4061 ms,
and poster/settings activation 10.9 ms. The latter does **not** include decoder
readiness. Repeated imports and deferred browser reclamation affected memory;
these process totals do not establish a leak or a memory plateau.

A real inactive-window/minimize attempt left `document.visibilityState` visible
in this automated desktop setup; that attempt kept playing and is not hidden-tab
proof. Explicitly labeled visibility emulation exercised the production listener
and stopped playback. Real desktop hidden-tab CPU remains unverified. The
redundant post-recording generic Events workload had an incorrect screen
assumption and was stopped; its partial rows are excluded. Earlier variable or
failed setup runs remain diagnostic evidence, not accepted measurements.

Transient logs, source clips, JSON and captures are under `/tmp/thoughts-video-spike`.
The durable policy is [Media handling and privacy](media-handling-policy.md).
New video/poster assets are unencrypted local display copies; originals are not
archived and existing assets are not rewritten. The intermittent black rectangles
remain unresolved. Linux browser verification does not establish macOS/Windows
runtime support.


Reproduce the video mode on a disposable stress demo, with a pre-generated
synthetic clip and no competing builds/tests:

```sh
THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 \
THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/synthetic-720.mp4 \
THOUGHTS_PERF_IDLE_SAMPLES=1 \
node internal/browserterm/client/background-performance.mjs \
  http://127.0.0.1:PORT APP_PID /tmp/video-results.json
```

`THOUGHTS_PERF_RECORDING=1` additionally requires an explicitly configured
synthetic PCM consumer host; it must not open a physical microphone. Native
one-time imports can be repeated independently with
`THOUGHTS_VIDEO_BENCH_FIXTURE=/tmp/synthetic-720.mp4 go test ./internal/visual -run '^$' -bench '^BenchmarkPrepareVideo$' -benchmem -count=5 -benchtime=1x`.
FFmpeg/ffprobe must be on PATH. Compile separately when using OS CPU/RSS tools.


A separate matched 1200×850 draft probe used 55 typed keys and five resize pairs
(650×600 ↔ 1200×850), with no competing builds/tests. Static/video typing median
was 12.4/13.3 ms; pooled p95 was 24.4/41.5 ms (+17.1 ms). This is an observed
responsiveness cost worth flagging, although it is not the Events navigation
metric and is not a significance claim. Resize event-to-first-row-mutation
ranged 0.5–1.8 ms static and 0.4–1.7 ms video; this does not measure complete GPU
paint. No timeouts occurred. Screenshots were retained and reviewed separately;
timing assertions alone do not establish visual correctness.


## Full traversal: viewport-only Thought formatting

The follow-up compares PR head `38b0aee436d420348d9808455287b52cfc69d815`
with `e61797fbf65b0d3778ecd3c9b5ff0748deafe571`. The original-head worktree
receives only the identical benchmark/test fixtures, not production changes.
The same migrated stress seed contains 721 Events and 20,000 Thoughts; its
ongoing Event contains 200 Thoughts. Seed DB SHA-256 before app use:
`9769c81d89fb24c488a476bb4f124166dc9b7500a58bd95ba6d8d9a59bec710e`.
The source clip is the identical synthetic eight-second 720p Baseline fixture
identified above. The prepared output hash is recorded in each run.

The previous video probe measured the first DOM mutation and mostly day changes.
The new opt-in interaction mode verifies the requested selection or completed
screen transition through xterm's public buffer/render callback. It walks the
whole day's 18 cards in both directions, opens/collapses the ongoing Event,
traverses all 200 Thoughts newest-to-oldest-to-newest (398 moves, including page
fetches), changes days, and samples All Thoughts scrolling. All Thoughts is a
bounded 30-record round trip, not a traversal of all 20,000 records.

Static control uses the video's exact poster. Paused and playing controls use
the same selected prepared clip at 100% speed. Five repetitions rotate scenario
order at 1200×850; one smaller walkthrough per state uses 650×600. Reload startup
and immediate actual Apply → navigation are recorded separately from settled
playback. The initial metadata viewport describes the launched browser window;
these workload dimensions are explicitly set by the probe. Native/backend
response arrival and subsequent render timings are separate; arrival stamps
include transport and can include interleaved terminal output, not isolated SQL
time. The later animation callback is a rendering opportunity, not proof of GPU
presentation. Diagnostic traces are separate from accepted timing runs.

The reference remains the i5-8250U (8 logical CPUs), Linux amd64, Go 1.25.14,
Brave 148.0.7778.217 and FFmpeg 6.1.1. Revisions run separately with no competing
builds/tests. CPU percentages use one logical CPU. These five runs do not establish
statistical significance.

### Hotspot and selected change

A temporary owned native host recorded a CPU profile during the same full
traversal; it added no application endpoint or permanent instrumentation. Of
8.69 CPU-seconds sampled over 88.46 seconds, cumulative
`renderBrowseThoughtsView` accounted for 33.95% (2.95 seconds), including 32.45%
in `SummaryPreview`. The renderer formatted up to 150 resident previews before
slicing out the viewport. ANSI styling, Unicode width calculation, copying and
allocation dominated that unnecessary work.

The retained production change formats only previews intersecting the viewport,
then clips partial rows exactly as before. Paging/selection state, statistics,
queries, distribution geometry, video quality and playback remain unchanged.
There is no cache or new renderer. Independent literal partial-row coverage and
existing navigation/geometry tests pass before and after.

A browser trace also showed document-root layout/painting. A four-line CSS row
containment experiment was stopped during its third repetition because the
partial samples showed no convincing benefit. It was discarded. Those partial
results are diagnostic evidence, not a completed five-run comparison or a claim
of a significant regression. Failed initial probe setup attempts (heading lookup,
entity timing and Home/End selection assumptions) are retained and excluded.

### Native measurements

Both benchmark suites use five one-second runs with compilation and fixture
setup outside timing. The new summary fixture has 150 records plus Create,
80-code-point Unicode previews plus ellipsis and a central alternating selection without data
commands. The existing Timeline benchmark keeps its original fixtures/sizes.
Values below are medians; byte/allocation values are per operation.

| Summary window | Before ns/op | After ns/op | Time delta | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| 80×27 Render | 4,784,460 | 275,737 | −94.2% | 366,463 → 24,090 | 7,399 → 421 |
| 80×27 Navigate | 4,732,980 | 300,056 | −93.7% | 366,543 → 24,178 | 7,403 → 425 |
| 140×45 Render | 4,635,074 | 457,674 | −90.1% | 313,763 → 36,992 | 6,499 → 631 |
| 140×45 Navigate | 4,762,801 | 485,060 | −89.8% | 313,850 → 37,071 | 6,503 → 635 |

| Timeline | Before ns/op | After ns/op | Time delta | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| 80×27 Render | 876,037 | 866,017 | −1.1% | 83,660 → 83,644 | 2,140 → 2,140 |
| 80×27 Navigate | 1,358,848 | 1,390,540 | +2.3% | 143,436 → 143,393 | 3,830 → 3,830 |
| 80×27 Expanded | 1,934,892 | 855,678 | −55.8% | 186,965 → 118,757 | 4,901 → 2,923 |
| 40×20 Render | 497,756 | 500,461 | +0.5% | 70,914 → 70,951 | 1,963 → 1,963 |
| 40×20 Navigate | 951,161 | 925,154 | −2.7% | 128,106 → 127,877 | 3,424 → 3,424 |
| 40×20 Expanded | 1,807,395 | 639,565 | −64.6% | 154,692 → 80,846 | 4,523 → 2,315 |

### Browser interaction and resource results

Each latency cell is **median/p95 before → after**, in milliseconds: the
median of five per-run medians/p95s, not a claim of statistical significance.
The prepared MP4 was byte-identical on both revisions: SHA-256
`5dd55f91046d34c34aa867eb6a542b28509210fee739a13a3df50aff996b0924`.
Both runs reported `VaapiVideoDecoder` / platform decoding.

| 1200×850 workload | Static | Paused | Playing 100% |
| --- | --- | --- | --- |
| Full-day Event selection | 16.9/38.0 → 18.6/32.3 | 19.8/33.2 → 19.6/30.9 | 20.5/28.3 → 20.4/28.6 |
| Event expansion/collapse | 20.7/37.8 → 19.6/32.4 | 28.6/38.3 → 24.2/32.0 | 30.1/32.8 → 29.7/32.1 |
| Full 200-Thought traversal | 19.7/34.1 → 20.9/33.0 | 23.2/33.6 → 18.5/31.8 | 23.4/29.7 → 21.2/32.0 |
| Day navigation | 21.5/33.7 → 25.9/41.6 | 25.1/37.1 → 25.2/37.8 | 25.3/37.7 → 25.4/37.9 |
| All Thoughts round trip | 19.5/27.3 → 16.8/27.3 | 14.8/26.5 → 16.1/26.8 | 16.9/29.3 → 16.2/25.3 |

Latency did not improve universally. Static day-navigation p95 rose 7.9 ms,
and playing expanded-Thought p95 rose 2.3 ms while its median fell 2.2 ms.
Startup also remains variable: reload-selection p95 was 31.2 → 32.9 ms static,
32.2 → 28.2 paused, and 33.2 → 36.2 playing. Immediate Apply-selection p95 was
28.2 → 31.9 ms. The refactor does not establish that initial lag is eliminated.
Native work is substantially cheaper; terminal frame scheduling, transport and
browser rendering remain part of end-to-end latency.

The initial single narrow pass appeared worse by more than 10 ms in several
workloads (selection 26.5 → 43.5 ms p95; full Thought traversal 30.1 → 42.1;
day navigation 30.9 → 46.6). That prompted a separate **five-run** playing-video
comparison at 650×600 using the same fixtures and full traversals:

| 650×600 repeat | Median before → after | p95 before → after |
| --- | ---: | ---: |
| Full-day Event selection | 18.6 → 17.3 | 28.4 → 28.0 |
| Event expansion/collapse | 24.1 → 19.4 | 37.3 → 31.1 |
| Full 200-Thought traversal | 20.7 → 19.7 | 34.1 → 30.0 |
| Day navigation | 21.5 → 20.7 | 31.3 → 31.0 |
| All Thoughts round trip | 19.8 → 19.2 | 29.7 → 27.9 |

The greater-than-10-ms increase did not repeat in that follow-up. Both the
initial result and repeats are retained; five runs do not prove it can never
occur. All measured transitions reached their expected selections/endpoints.
All five playing-video frame-counter snapshots reported zero dropped frames on
each revision; this is not a universal playback guarantee.

Resource values are medians of the five desktop repetitions. Sequence CPU is
CPU-milliseconds for the same automated interaction sequence, including browser
instrumentation and playback during it, not isolated decoder cost or steady
utilization. Total CPU is the median of per-run browser+native sums.

| State | Native sequence CPU ms | Browser sequence CPU ms | Total sequence CPU ms | Idle browser CPU |
| --- | ---: | ---: | ---: | ---: |
| Static | 7,080 → 5,290 | 13,680 → 14,440 | 20,760 → 19,690 | 1.62% → 1.48% |
| Paused | 7,400 → 5,290 | 13,900 → 14,530 | 21,300 → 19,820 | 1.49% → 1.61% |
| Playing 100% | 7,410 → 5,650 | 16,720 → 17,220 | 24,050 → 23,110 | 25.81% → 24.21% |

Native sequence CPU fell 24–29%; total sampled CPU fell about 4–7%. Browser
sequence CPU rose about 3–6%, so no browser CPU improvement is claimed. Native
RSS remained similar (static 41.55 → 40.16 MiB, paused 41.92 → 41.12, playing
41.56 → 40.71); browser PSS was 330.22 → 330.89, 332.78 → 329.04, and
344.16 → 341.21 MiB respectively. Lower allocation does not imply immediate RSS
reclamation or prove a memory plateau/leak absence.

**The additional playback CPU target below 10% remains missed.** Idle playback
is still approximately 24–26% of one logical CPU; the media path did not change,
so the small idle difference is not attributed to this refactor. Video quality,
limits, speed/pause semantics and poster ownership are unchanged. The prior
intermittent black-rectangle report remains unresolved. Opaque cell regions were
visible in both baseline and optimized walkthroughs; their relationship to the
earlier report was not established, and no compositor fix is claimed.

### Reproduction and limits

Generate/migrate the disposable stress fixture and compile before measurement.
Run the two revisions separately with FFmpeg/ffprobe on PATH:

```sh
THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 \
THOUGHTS_PERF_INTERACTIONS=1 \
THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/synthetic-720.mp4 \
node internal/browserterm/client/background-performance.mjs \
  http://127.0.0.1:PORT APP_PID /tmp/interactions.json

go test ./internal/tui/thoughts -run '^$' \
  -bench '^BenchmarkThoughtSummaryWindow$' -benchmem -count=5 -benchtime=1s
go test ./internal/tui/events -run '^$' \
  -bench '^BenchmarkTimeline$' -benchmem -count=5 -benchtime=1s
```

`THOUGHTS_PERF_INTERACTION_NARROW_ONLY=1` runs five playing-video repetitions at
650×600. Workload rows now record their viewport explicitly.
`THOUGHTS_PERF_INTERACTION_SMOKE=1` reduces repetitions for workload validation;
`THOUGHTS_PERF_TRACE=1` records a diagnostic browser trace. Neither substitutes
for unprofiled repeated timings. Logs, profiles, discarded experiments and
captures are retained under `/tmp/thoughts-video-optimization`.

Desktop/narrow expanded captures and immediate Apply → entity Left/Right were
reviewed on both revisions with synthetic media. The terminal socket remained
unchanged. All 64 Chromium/Firefox acceptance cases and the visual/framing/PCM/
microphone unit suites passed, including import during recording. `make quick`
and `make ci` passed with the native processor enabled; the latter includes
fresh unit/integration tests, build, pinned migration CLI and the race detector.

The probe accepts an external synthetic clip and records actual prepared
geometry/frame counters, so later 1080p work can reuse it. At this revision, stored bounds
remained 720p-class; an uploaded 1080p source still becomes a bounded 720p-class
derivative. These results do not establish 1080p readiness. No long one-time
import benchmark was repeated because the media pipeline did not change.

## New-upload trimming (2026-10-16)

The trimming implementation is `0b70c98bd5f0b8e7cdbde7017bd75443401ee5fb`,
on merge-base `357737d`. Native timing used the same implementation before its
commit; subsequent changes concerned control/ownership checks and documentation,
not the measured encoder or persistence pipeline. These are one-time import
measurements, not recurring playback or 1080p evidence.

A source can be 1–120 seconds; selected footage remains 1–30 seconds. The existing
128 MiB / 4096-axis / 9MP / 60 FPS / four-billion whole-source pixel budget still
applies. Stored quality, 720p-class bounds and 24 FPS cap are unchanged. Only an
explicit Preview loop encodes. Draft cut changes keep the last acknowledged loop;
Done/Apply use that loop, not unpreviewed markers. Apply copies its exact video
and poster bytes, without starting FFmpeg. An identical Preview request reuses
that editor's acknowledged files and decoded player. This is temporary edit
ownership, not an application playback cache or an original archive.

### Native import phases

Reference machine: Intel i5-8250U, eight logical CPUs, Linux amd64, Go 1.25.14,
FFmpeg/ffprobe 6.1.1. Synthetic `testsrc2`, 1280×720, 24 FPS, SDR Rec.709 sources
were generated before timing. The 30-second fixture is 8,913,407 bytes (SHA-256
`670bbb738fe532b2175d95eb9fe5b61503c2b98f31eb700c9d4237a27ff16b23`);
the two-minute source is 35,503,362 bytes. No other test/build/probe ran during
these measurements. Each revision was compiled separately; timing used the test
binaries, excluding Go compilation. Medians of three one-iteration runs:

| Same 30-second full preparation | ns/op | Go B/op | Go allocs/op |
| --- | ---: | ---: | ---: |
| Merge-base | 9,748,841,364 | 73,217,488 | 769 |
| Trimming pipeline | 9,271,491,610 | 20,958,744 | 762 |
| Observed change | −4.9% | −71.4% | −0.9% |

The retained result is file-backed rather than a pair of Go byte slices. FFmpeg
still performs essentially the same encoding work. Do not interpret three runs
as statistical significance or a general CPU speedup. GNU time maximum RSS was
119,928 versus 119,940 KiB: effectively unchanged. This is the largest individual
process peak reported for the command, not summed simultaneous app/child memory.
Go allocation figures do not include FFmpeg allocations.

| Two-minute source; ten-second prepared loops | ns/op | Go B/op | Go allocs/op |
| --- | ---: | ---: | ---: |
| Stage/validate once | 88,247,937 | 110,472 | 301 |
| Preview 0–10s | 3,380,172,890 | 20,844,392 | 464 |
| Preview 60–70s | 3,780,644,832 | 20,844,632 | 460 |
| Preview 110–120s | 3,624,381,809 | 20,843,160 | 456 |
| Apply prepared 0–10s | 25,761,488 | 17,004,696 | 81 |

Apply uses the existing file-write/fsync boundary and an in-memory settings store
in this native benchmark; real SQLite/UI persistence is included below. It still
uses bounded transient buffers to call that boundary. Phase-command maximum RSS
was 120,072 KiB. Late seeking decodes a bounded preceding GOP; it does not encode
the preceding source footage. No processor runs while merely moving cut points.

Reproduce with synthetic fixtures of the stated geometry/duration:

```sh
THOUGHTS_VIDEO_BENCH_FIXTURE=/tmp/source-30.mp4 go test ./internal/visual \
  -run '^$' -bench '^BenchmarkPrepareVideo$' -benchmem -count=3 -benchtime=1x
THOUGHTS_VIDEO_BENCH_FIXTURE=/tmp/source-120.mp4 go test ./internal/visual \
  -run '^$' -bench '^BenchmarkVideoTrim$' -benchmem -count=3 -benchtime=1x
```

### Browser import and reuse

The existing disposable stress host (721 Events / 20,000 Thoughts) and headed
Brave 148.0.7778.217 at 1920×1080 measured five imports of the same 30-second clip.
The imported image control remains the existing synthetic 1200×850 PNG. The
probe's import-only mode now skips the unrelated long idle/navigation matrix for
video as well as images. Medians:

| Operation | ms |
| --- | ---: |
| Choose/upload + first explicit preparation + return to Visual | 12,927.8 |
| Explicit identical-cut preview reuse | 631.4 |
| Apply → returned TUI and visible selected player | 353.2 |
| Decoded-poster activation within Apply | 11.5 |

These include picker/control automation and terminal repaint. They are not raw
HTTP or decoder timings. Preview now includes the native encode that older
versions deferred to Apply; the old raw-file preview is not an equivalent task.
Settings-only reuse remains separate from import. Browser summed RSS/PSS at the
five video completions had medians 962.7/460.4 MiB (RSS range 916.2–993.8 MiB);
app RSS median was 57.3 MiB. Shared browser pages are counted in RSS. This is not
a claim of constant memory or a leak proof; the focused tests independently
check URL/player retirement.

Use the existing probe with `THOUGHTS_PERF_IMPORT_ONLY=1` and
`THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/source-30.mp4`. It records identical-cut reuse
separately. Transient fixtures, raw native results, JSON, host source and logs
are under `/tmp/thoughts-trim-measurements`; visual captures are under `/tmp`.
The previously recorded recurring playback budgets and missed targets above
remain prior evidence, not new guarantees from these import measurements.

### Recurring interaction smoke

A **single** existing interaction-probe run at `0b70c98` used a synthetic
1280×720 / 24 FPS / 8.083-second clip, static control from its exact poster,
paused video, and playing video. Desktop 1200×850 and narrow 650×600 traversed
the full day's Events and all 200 Thoughts in the ongoing Event, in both
directions, plus expansion/collapse and day navigation. The selected file's
identity is recorded in `interactions-smoke.json`. This is workload validation,
not a repeated before/after comparison or a statistical regression conclusion.

| Desktop p95, ms | Static | Paused | Playing 100% |
| --- | ---: | ---: | ---: |
| Full-day Event selection | 27.3 | 28.4 | 24.2 |
| Expansion/collapse | 40.6 | 30.7 | 42.3 |
| Full 200-Thought traversal | 45.4 | 31.0 | 34.2 |
| Day navigation | 32.2 | 31.4 | 48.6 |
| All Thoughts round trip | 32.0 | 29.7 | 15.1 |

Eight-second idle windows reported browser CPU 1.49% static, 1.61% paused and
27.30% playing; native CPU was 2.11%, 1.98% and 2.11%, respectively, relative to
one logical CPU. **The additional CPU target below 10% was missed** (+25.81
points). The day-navigation p95 difference also exceeded 10 ms in this one pass;
it does not establish that the difference is repeatable or caused by trimming.
The earlier five-run evidence and its variability above remain relevant.
Brave reported `VaapiVideoDecoder` / platform decoding; this does not attribute
all CPU to decoding or prove GPU activity from browser capability alone. The
playing sequence's frame counters reported 701 frames and zero dropped frames.

This follow-up did not change encoding quality, playback speed defaults or
terminal rendering to chase those numbers. Pausing, selecting a smaller or
lower-FPS source, slower playback, or retaining an image remain the simple
tradeoffs. It does not establish 1080p readiness. Desktop/narrow trim and main
UI captures were inspected; no compositor fix is claimed. Chromium/Firefox
image, voice, clipboard, framing and video acceptance passed (70 cases, four
fixture-specific onboarding skips), followed by two onboarding-enabled Visual
cases and a fresh four-case trim/retirement check. Those checks are functional
acceptance, not CPU benchmarks.

The same single-pass narrow p95s were:

| 650×600 p95, ms | Static | Paused | Playing 100% |
| --- | ---: | ---: | ---: |
| Full-day Event selection | 21.0 | 41.8 | 27.5 |
| Expansion/collapse | 41.3 | 32.3 | 41.4 |
| Full 200-Thought traversal | 26.8 | 41.2 | 27.6 |
| Day navigation | 26.9 | 46.9 | 32.4 |

After measurement, `ab79197` added a constant-time output postcondition: a valid
prepared file must also match the requested span within one frame (plus timestamp
rounding). This rejects incomplete processor output; it does not change the
encoder, scaling, seeking, file copies or playback. Its focused native timing
regressions pass; the long benchmarks were not repeated for that comparison.

## Video quality through 1080p60

The quality extension keeps **Efficient** as the initial preference (720p30,
3 Mbps VBV / 16 MiB stored video). High quality permits 1080p30 at 6 Mbps /
32 MiB; Maximum permits 1080p60 at 10 Mbps / 64 MiB. These are ceilings, not
upscaling targets. Portrait footage uses the same longest-axis/pixel budget.
Lower and fractional source rates stay exact through preparation. Playback
speed remains a separate native browser control; changing it does not encode.
The [media policy](media-handling-policy.md#looping-video-backgrounds) describes
source-window work limits, H.264 levels, independent validation and privacy.

### Qualification fixtures and method

Measurements use the reference ThinkPad T480s, i5-8250U / UHD Graphics 620,
eight logical CPUs, Linux 6.18.7, Go 1.25.14 and FFmpeg 6.1.1. Sources are
generated/downloaded before timing; test binaries and the disposable browser
host are compiled separately. No concurrent Go tests, asset builds or other
performance probes run during the timed samples. CPU percentages below are relative to **one logical CPU**.

The ambient fixture comes from [NPS Video: Michael Quinn, scarlet monkey
flowers at Hermit Creek](https://www.nps.gov/media/video/view.htm?id=39E50741-155D-451F-67CACF2694A2F3E7).
That page explicitly identifies the unmarked NPS work as public domain. Credit
does not imply endorsement. The source is 1920×1080, 24000/1001 FPS, 32.198833s,
25,459,672 bytes, SHA-256
`0caaa644b7662938d0ed0cdfc32621962fa824c955ae01f3aa098c880c1ecac1`.

Matched 30-second 1920×1080/60 input fixtures make resolution/rate costs
comparable:

| Input | Bytes | Frames | SHA-256 |
| --- | ---: | ---: | --- |
| Synthetic `testsrc2` motion | 49,075,969 | 1800 | `dad50816e87fa6267ea0a2f9bb3841d37bbae77efbaea3dd5ec7a95de9e4b791` |
| Ambient, controlled 60 FPS copy | 41,720,954 | 1800 | `f72eb14e82a9f09a7246b5e16a6a172c00717dfa11ade7276687950ab2946d2f` |

The ambient copy **repeats native 23.976 FPS frames only for the benchmark**;
it neither interpolates motion nor establishes natural 60 FPS capture quality.
Production preparation never raises a source's declared FPS. Native/browser
tests separately cover native 23.976/29.97/59.94 streams. The reproducible recipe
is `sh scripts/video-quality-fixtures.sh /tmp/new-quality-fixtures`; generated
binary media stays outside Git. FFmpeg/version differences can change hashes.

`BenchmarkPrepareVideo` accepts `THOUGHTS_VIDEO_BENCH_QUALITY` and records actual
encoded dimensions, exact FPS, frame count, bytes and SHA-256. The browser probe
accepts `THOUGHTS_PERF_VIDEO_QUALITY` and
`THOUGHTS_PERF_QUALITY_STUDY=1` with the existing interaction mode. That mode
measures static/paused/playing repetitions with the same prepared bytes, then
normal/50%-speed playback, synthetic microphone capture, pause, reduced motion
and typing. `THOUGHTS_PERF_SAMPLES` selects 1–5 repetitions (default five). This
study uses three repetitions; the already-completed motion/Efficient navigation
series retains its five runs. `THOUGHTS_PERF_CONDITIONS_ONLY=1` reuses that
completed navigation series for a conditions-only follow-up. Only the first
import in each probe invocation encodes; subsequent Apply interaction samples
are explicitly labeled settings-only. It retains the full Event and
200-Thought traversals, day navigation, All Thoughts round trip, resource and
native-player counters. Navigation uses a 1200×850 viewport, plus a 650×600
pass for each mode; these are not full-HD-viewport composition measurements.
Decoder properties are observations, not inferred from codec capability.

For example, after generating the fixtures and starting a disposable migrated
stress host (never the personal app on port 7777):

```sh
go test -c -o /tmp/visual-quality-bench ./internal/visual
THOUGHTS_VIDEO_BENCH_FIXTURE=/tmp/new-quality-fixtures/motion60.mp4 \
THOUGHTS_VIDEO_BENCH_QUALITY=maximum \
  /usr/bin/time -v /tmp/visual-quality-bench -test.run '^$' \
  -test.bench '^BenchmarkPrepareVideo$' -test.benchmem -test.count=3 -test.benchtime=1x

THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 THOUGHTS_PERF_INTERACTIONS=1 \
THOUGHTS_PERF_QUALITY_STUDY=1 THOUGHTS_PERF_PREVIOUS_DAY=1 \
THOUGHTS_PERF_SAMPLES=3 \
THOUGHTS_PERF_VIDEO_QUALITY=maximum \
THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/new-quality-fixtures/motion60.mp4 \
  node internal/browserterm/client/background-performance.mjs \
  http://127.0.0.1:PORT APP_PID /tmp/quality-maximum.json
```

Repeat each tier with both fixture files. The browser study passes a local file
path to the real picker; it avoids including Playwright's large in-memory file
transfer in import timing. Timing is keydown to the matching terminal render,
with animation-opportunity timing separately retained; it excludes automation
key delivery and is not a physical-keyboard-to-GPU-presentation measurement.

The disposable host uses UTC and a fixed preceding-day stress seed, selected
through the real Left key with `THOUGHTS_PERF_PREVIOUS_DAY=1`. This prevents
midnight changing the workload during the study. Reduced-motion samples start
a fresh page to clear the deliberately supported session Play override. Other
condition changes stay in one terminal connection; the synthetic audio stream
uses its separate existing connection. Personal app/data processes are untouched.
To reproduce that fixed seed, create `TEMP demo_clock(at INTEGER NOT NULL)` and
insert `unixepoch('now','start of day','-30 minutes')` before executing
`scripts/demo_stress.sql` in the **same SQLite connection**, then run the host
with `TZ=UTC`. The seed owns and drops that temporary clock. The resulting
selected day contains 24 Event entries, including the ongoing Event with 200
Thoughts; All Thoughts contains 20,000 records. The disposable host discards
synthetic PCM through the existing injected consumer, so microphone numbers
include capture/transport but **exclude speech recognition**.

### Native preparation and encoded results

The processor measured is `09d8302`, including active decoded-frame budget
cancellation, packet-envelope checks and full output decoding. Each cell is the
median of three one-iteration compiled-binary runs. Preparation includes staging,
probing, encoding, independent verification, poster generation and cleanup;
fixture loading and artifact capture are outside the benchmark timer. GNU time
CPU includes those extra command phases and all child processes. FFmpeg peak RSS
is sampled every 100 ms across its sequential phases, **not** combined app/child
memory or a hard memory limit. The machine was on AC with the powersave governor.

| Footage | Tier | Preparation seconds | Command CPU seconds | FFmpeg peak RSS MiB |
| --- | --- | ---: | ---: | ---: |
| Motion | Efficient | 21.04 | 21.18 | 128.18 |
| Motion | High quality | 28.27 | 28.56 | 187.90 |
| Motion | Maximum | 43.12 | 43.46 | 190.70 |
| Ambient | Efficient | 24.81 | 25.02 | 130.80 |
| Ambient | High quality | 36.38 | 36.65 | 190.54 |
| Ambient | Maximum | 49.58 | 49.87 | 193.11 |

Every run completed below the existing two-minute per-operation deadline; it
was not increased. Three samples are qualification evidence, not a timing
guarantee for other machines or a claim of statistical significance. The test
process's sampled peak RSS medians were 70.5–77.1 MiB at Efficient,
90.5–118.5 MiB at High quality and 118.4–125.5 MiB at Maximum. Those include
the benchmark's loaded fixture and untimed artifact copies; they are separate
from FFmpeg memory and are not an estimate of the streaming app's import RSS.

All outputs independently probe as Constrained Baseline H.264, one reference,
`yuv420p`, square pixels and limited-range Rec.709, one video stream with no
audio. They are exactly 30 seconds. Efficient is 1280×720/30 FPS/900 frames at
Level 3.1; High quality is 1920×1080/30 FPS/900 frames at Level 4.0; Maximum is
1920×1080/60 FPS/1800 frames at Level 4.2. All three repetitions produced the
same SHA-256 for each derivative:

| Footage | Tier | Video bytes | SHA-256 |
| --- | --- | ---: | --- |
| Motion | Efficient | 5,549,435 | `792acfaa5f64fabfe7ec56d35815c794e7d766a2c4aceb4b4a2d63950a58b346` |
| Motion | High quality | 21,822,744 | `1fa6ae2390a156320f58da2e54e1326f27bc046af77a2076a20f387bd3bd0de1` |
| Motion | Maximum | 29,135,633 | `3634d259d314991a84fed2a7eed11c6ad62c2860af954cccc743c3fde2cbb1b7` |
| Ambient | Efficient | 11,551,092 | `10a7f425947400394276eb60c32605ea47e4feaa3eb84b9be8cea94829be5418` |
| Ambient | High quality | 23,099,214 | `36354dc5384e1bb5b81308eb23aaa81bab398788616b1c182656a574d3b2c8f9` |
| Ambient | Maximum | 29,336,681 | `01e27f30d4d655f680c96f63bff5681890f660bcdcf56380a17e2d611f2df743` |

The starting 3/6/10 Mbps VBV and 16/32/64 MiB budgets are retained. CRF does not
fill the budget: motion Efficient averages about 1.48 Mbps. VBV permits bounded
bursts from its two-second reservoir; a short clip's average can therefore be
slightly above the named maximum rate. Validation checks that actual packet
envelope and stored bytes, not a strict average-rate assumption. A fully used
10 Mbps/20 Mbit envelope over 30 seconds also explains why Maximum needs more
headroom than the old 16 MiB cap.

SSIM against each corresponding **pre-encode scaled/rate-limited** reference is
0.993723/0.995164/0.991909 for motion and 0.953872/0.974124/0.980777 for ambient,
in tier order. This measures re-encoding loss, not the detail removed by scaling
or temporal samples removed by FPS reduction, and cannot rank perceptual quality
across resolutions. Textured footage remains lossy, especially at Efficient.
These two clips are a bounded qualification set, not broad codec-quality proof.

### Browser playback and interaction

Brave 148.0.7778.217 ran headed on the same T480s. Each condition below is the
median of three eight-second samples. Browser CPU covers the processes reported
by CDP; memory is their summed PSS, avoiding the double counting in summed RSS.
Native CPU is the app host. Synthetic microphone capture includes transport but
not speech recognition. Values describe these fixtures and viewports, not a
guarantee for other media, desktop activity or machines.

| Footage | Tier | Normal CPU | Half-speed CPU | With microphone CPU | Browser PSS MiB | Native CPU |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Motion | Efficient | 23.8% | 15.9% | 40.0% | 291.8 | 2.0% |
| Motion | High quality | 26.9% | 16.2% | 41.5% | 379.2 | 2.2% |
| Motion | Maximum | 41.6% | 25.5% | 58.1% | 344.4 | 1.9% |
| Ambient | Efficient | 26.2% | 14.5% | 42.1% | 359.0 | 2.1% |
| Ambient | High quality | 28.8% | 15.1% | 42.7% | 391.5 | 2.1% |
| Ambient | Maximum | 51.3% | 26.1% | 56.8% | 348.9 | 2.1% |

The table below gives median/p95 **milliseconds**, taking the median of each
run’s median and p95. Playing-mode navigation covers the full stress workloads;
normal-speed typing uses three eleven-character sequences. These are render
latencies, not physical-keyboard-to-screen presentation measurements.

| Footage | Tier | Event selection | Expand/collapse | 200 Thoughts | Day changes | All Thoughts | Typing |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Motion | Efficient | 15.0/30.1 | 18.6/34.4 | 17.5/31.2 | 166.8/185.5 | 17.0/29.9 | 14.0/14.8 |
| Motion | High quality | 18.0/28.0 | 21.5/31.1 | 21.4/31.2 | 169.8/186.3 | 19.7/30.1 | 19.9/27.9 |
| Motion | Maximum | 19.9/30.7 | 23.2/31.9 | 22.7/30.5 | 174.3/195.8 | 14.9/28.2 | 16.1/30.1 |
| Ambient | Efficient | 15.9/28.2 | 19.2/36.9 | 18.2/30.5 | 169.4/185.2 | 18.6/38.1 | 13.9/25.4 |
| Ambient | High quality | 17.2/28.1 | 20.3/31.6 | 19.7/30.1 | 169.2/185.2 | 18.8/39.2 | 17.4/30.5 |
| Ambient | Maximum | 19.9/27.2 | 25.7/32.2 | 22.0/32.9 | 181.1/210.5 | 14.7/27.6 | 13.7/27.9 |

Native player counters also retain the drops instead of equating successful
playback with perfect delivery. Entries below are **dropped/total frames**;
paused and reduced-motion samples advanced no frames.

| Footage | Tier | Normal samples | Microphone samples | Half-speed samples | Playing navigation |
| --- | --- | ---: | ---: | ---: | ---: |
| Motion | Efficient | 0/727 | 1/726 | 0/364 | 3/5178 |
| Motion | High quality | 0/725 | 1/725 | 0/363 | 10/3088 |
| Motion | Maximum | 0/1453 | 1/1450 | 1/726 | 15/6174 |
| Ambient | Efficient | 0/727 | 2/727 | 1/364 | 3/3146 |
| Ambient | High quality | 0/726 | 1/725 | 3/364 | 12/3116 |
| Ambient | Maximum | 0/1452 | 1/1450 | 0/725 | 16/6254 |

The last column uses each navigation pass's cumulative player counters, including
startup and settling. It is not a count restricted to timed keystrokes. No
normal-speed steady sample dropped a frame; small drops occurred during some
microphone, slow-speed and navigation passes.

The static/paused controls put these latencies in context. Their expanded-Thought
p95 medians ranged from 27.3–33.1 ms; playing medians were 30.1–32.9 ms. Day
changes were slower with video: Maximum reached 195.8/210.5 ms p95 for
motion/ambient, versus its static controls' 175.2/174.4 ms and Efficient's playing
185.5/185.2 ms. These runs do not show a broad typing/selection failure, but they
do show additional day-navigation cost. No unrelated rendering optimization was
introduced. Higher tiers remain opt-in, and the quality control explains their
potential CPU cost.

Pause/reduced-motion median browser CPU was 0.7–1.5%, with no advancing video
frames. During repeated playing-navigation passes, browser PSS ranged from
376.9–405.1 MiB and native RSS from 38.4–46.4 MiB. Browser series did not grow
monotonically; the largest native increase was approximately 43.1 → 46.3 →
46.4 MiB. This short qualification is not a long-duration leak test. Fresh
condition-only follow-ups and browser caches affect the table's PSS medians;
they do not establish a monotonic memory ranking among tiers. Terminal connection
counts stayed at one throughout each condition page, including microphone and
settings transitions; reduced motion deliberately uses a fresh page.

CDP reported `VaapiVideoDecoder` and `kIsPlatformVideoDecoder=true` for the
observed Brave players at all tiers. This is observed decoder evidence, not an
inference from codec support and not a claim about every Linux configuration.
Firefox 151.0 also decoded the ambient 1080p60 derivative in a headless 31-second
check spanning the 30-second loop boundary: 1,863 additional frames, zero reported
drops, and successful reduced-motion pause. Its hardware-decoder path and steady
CPU were not measured. macOS/Windows runtime video compatibility remains
unverified.

All six browser-prepared MP4 hashes matched the native derivatives above, with
actual width/height, exact FPS, duration and frame counts checked from preparation
replies. Desktop/narrow captures and the ambient posters were inspected. Earlier
benchmark-driver setup/focus failures are excluded from condition timings;
completed navigation series were retained instead of repeated. The driver now
waits for speed-label changes and playback acknowledgments before navigating.
Raw records, captures and fixture manifests remain under
`/tmp/thoughts-video-quality-review`; the tracked harness and fixture recipe allow
new measurements without personal media or app data.

The Maximum-import recording regression uses the real local-file picker path.
Playwright's in-memory multi-megabyte injection reproducibly blocked Chromium
long enough to exhaust the bounded worklet credits; the local-file path passed
with the same track, draft and connection assertions. Audio queue limits and
production capture behavior were not relaxed.

### Video preparation activity wave

The approved [activity-wave style](app-style-guide.md#approved-video-preparation-indicator)
uses a fixed braille curve with a fading light trail and bright head above
Options. Two copies of the curve are rendered once. Clipped windows move over
them using opposing CSS transforms, in 55 steps over approximately 4.58 seconds.
There are no animation-driven text rewrites, Go ticks, HTTP requests, or
WebSocket frames. The browser reserves vertical space only while preparation is
pending. Hiding the page, reduced motion, cancellation, disconnection, and
completion remove the animations.

On 2026-10-21, the T480s/i5-8250U/UHD 620 Linux reference machine was checked with
visible Brave 148.0.7778.217, a disposable seeded app, and the existing synthetic
1080p60 motion fixture. No test/build jobs ran alongside the measurements. The
probe used the existing harness's CDP process CPU and Linux PSS accounting.
Percentages below are fractions of **one logical CPU**, not the entire machine.

| Condition | Browser CPU | Native app CPU |
| --- | ---: | ---: |
| Wave active, preparation HTTP request held before FFmpeg (two 10-second windows) | 12.6–13.4% | 2.1–2.2% |
| Same view, static reduced-motion control (10 seconds) | 0.8% | 1.8% |
| Actual 30-second Maximum loop preparation and preview acknowledgement (47.0 seconds) | 6.3% | 1.3% |

Native app CPU excludes the FFmpeg subprocess. These are whole-browser costs,
not an isolated estimate of the wave's rendering instructions. The active and
static samples held the HTTP request to separate the indicator from encoding;
the real-preparation sample includes encoding, output validation, poster work,
and preview acknowledgement. Its lower browser average does not establish that
encoding makes the indicator cheaper.

The earlier implementation rewrote five braille text layers at 12 Hz and used
22.5–23.4% browser CPU in the same held-request comparison. Keeping text fixed
reduced that cost, but the animation is still measurable. It is limited to a
pending import, not an always-running decoration. A JavaScript timer driving the
same transforms did not improve the measured result, so the implementation keeps
the simpler CSS animation.

Separate two-second visual-step samples had an 83.3 ms median interval and
83.4–99.9 ms p95. The single Tab-to-Cancel focus sample during actual preparation
was 29.9 ms, with one WebSocket connection throughout. These short samples do
not establish a long-duration responsiveness or battery guarantee. Browser PSS
was 326–341 MiB in the active control windows and 377 MiB after preparing and
decoding the Maximum preview; renderer JS heap was approximately 4.8–5.1 MiB.
The prepared preview starts native video playback after acknowledgement, so its
subsequent CPU cost must not be mistaken for a still-running wave. The wave had
no further updates or active animations after completion.

Focused Chromium and Firefox acceptance covers normal/narrow layouts, resizing,
focus, reduced motion, hidden-page suspension, success, failure, cancellation,
and stale operation ownership. Temporary measurement scripts, JSON records and
captures are kept together under `/tmp/thoughts-sine-preview`; the retained
transform measurements are `performance-css-transforms.json`. They are local
review artifacts, not required runtime assets or a new performance framework.

## Long video sources

The long-source editor accepts compatible local MP4/MOV sources from one second
to four hours, up to **8 GiB**. Prepared backgrounds remain **1–30 seconds** at the
existing Efficient, High quality and Maximum ceilings. Selection opens a paused
browser-local File/object URL; Start is its sole source cursor. Moving Start
proposes the next 30 seconds, with a one-second minimum remaining at the source
boundary. End can shorten that proposal. The first explicit Preview loop streams
the entire File once to native staging; later previews reuse that staged input.
Apply copies the acknowledged derivative/poster, without another encode.

A transfer has a 20-minute total limit and 30-second idle-read limit. Source
container/probe validation starts its own two-minute deadline after transfer;
preparation keeps its existing separate two-minute deadline. The decoded-work
limits stay at four billion source pixels and 7,200 frames **per preparation**,
including keyframe preroll. Increasing source duration does not increase them.
Native container inspection skips media payload using 64-bit offsets. The bounded
source movie index ceiling increases from 8 to 32 MiB; an independently generated
two-hour 60 FPS H.264/AAC sample had an 11,863,918-byte index. Prepared-loop indexes
retain their 8 MiB ceiling. No complete-movie decode, transcode or thumbnail index
is needed to stage or browse a source.

Browser imports require zero-start video/container timestamps. A focused offset
fixture demonstrated why: Chromium initially reported currentTime=5 and showed
source content around 1.25 seconds after seeking to 6.25; Firefox normalized the
same file to zero and showed content at 6.25. Local detection/native verification
reject this disagreement, remove native staging, and give fixed export feedback.
Native source-relative trimming and its existing nonzero-offset regressions remain
supported. This MVP does not remux originals to repair their timelines.

### T480s observations

Measured on 2026-10-21–22 with the reference i5-8250U/UHD 620 ThinkPad T480s, Linux,
Go 1.25.14 and Brave/Chromium 148.0.7778.217. The headful browser reported a
1920×1080 requested viewport. Three final runs used a synthetic 1920×1080, 30/1 FPS,
7,200-second MP4: 216,000 frames, two-second keyframe interval, no B frames/audio,
and **2,835,203,027 bytes (2.64 GiB)**. SHA-256:
`a56a77e62d9a80c49060643db338d29f08da5c9d6edbf37b0ae3d7473008c678`.
Its ten-second high-motion seed was repeated with stream copy, outside timing.
The prepared late interval was 01:59:50–02:00:00, Efficient 1280×720 at 30/1 FPS,
300 frames and 10,000 milliseconds.

The final browser series ran after competing test/build processes finished, with
no local builds or tests during timing. These are three observations, not a
performance guarantee. Source hashes were streamed before timing; page-cache
state was not reset. Native measurements below came from an earlier series under
shared-machine load and are identified separately.

| Browser operation | Median | Observed range |
| --- | ---: | ---: |
| File selection → usable paused source | 163 ms | 155–223 ms |
| Direct source jump → completed seek, nine samples | 135 ms | 128–184 ms |
| Initial whole-source transfer, browser upload-complete notification | 15.08 s | 12.00–16.27 s |
| Transfer plus native validation/response | 15.15 s | 12.28–16.31 s |
| Native late-loop preparation/verification and response | 4.42 s | 4.39–5.21 s |
| Prepared preview loading/acknowledgement and TUI reply | 103 ms | 87–107 ms |
| Apply, including activation and return navigation | 292 ms | 181–409 ms |

Upload completion is a browser notification, not an authoritative processor-phase
boundary; subtracting it from response timing does not isolate FFprobe CPU time.
The earlier shared-load native benchmark measured container/probe validation at a median
76.8 ms. Native staging, including a real local-file copy, took a median 22.68 s
(range 18.98–32.00 s). Native ten-second preparation medians were 4.59 s at the
start, 4.56 s in the middle and 4.89 s at the end; Apply was 56 ms. This supports
late seeking within the fixed work budget for this indexed, short-GOP fixture;
it does not establish fast seeking for every supported container.

During each complete browser import/seek/prepare/Apply workload, browser CPU
increased by 12.66–14.38 CPU-seconds and native-host CPU by 6.79–7.71 CPU-seconds.
The host counter excludes processor children. Sampled host peak RSS was
32.4–36.7 MiB. Summed browser peak RSS was 729–853 MiB, which double-counts shared
pages; peak browser PSS was 265–348 MiB and ending PSS was 226–245 MiB. These are process observations, not
hard RSS bounds, and exclude kernel file caches. The earlier shared-load native benchmark
series, including children and untimed setup, consumed 45.46 user/14.60 system
CPU-seconds over 141.42 seconds; GNU time reported peak RSS of 119.8 MiB. Staging
allocated approximately 130–161 KiB of Go heap per operation rather than a
movie-sized byte slice. Peak native temporary input occupies one source's size,
plus bounded loop candidates/posters; a previous staged source is retired before
receiving its replacement. Normal Apply/Cancel/session retirement removed staging. The final browser series
sampled a peak 2,840,060,658 logical temporary bytes (2,840,068,096 allocated),
consistent with one source plus bounded output; no temporary files remained.

CDP reported `VaapiVideoDecoder` with `kIsPlatformVideoDecoder=true` for observed
players. This is an observed decoder selection, not a matched software-decoder
comparison or a guarantee for other machines. Steady playback, slowdown and
reduced-motion costs are unchanged architecturally; the earlier quality study
remains separate from these one-time import measurements.

### Reproduction and limits

Generate only disposable, non-private fixtures outside routine CI:

```sh
ffmpeg -f lavfi -i testsrc2=size=1920x1080:rate=30:duration=10 \
  -c:v libx264 -threads 2 -preset ultrafast -pix_fmt yuv420p \
  -b:v 3M -minrate 3M -maxrate 3M -bufsize 6M -x264-params nal-hrd=cbr \
  -g 60 -color_primaries bt709 -color_trc bt709 -colorspace bt709 \
  -color_range tv -movflags +faststart /tmp/seed.mp4
ffmpeg -stream_loop -1 -i /tmp/seed.mp4 -t 7200 -c copy \
  -movflags +faststart /tmp/two-hours.mp4

THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 THOUGHTS_PERF_LONG_SOURCE=1 \
THOUGHTS_PERF_IMPORT_ONLY=1 THOUGHTS_PERF_SAMPLES=3 \
THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/two-hours.mp4 \
  node internal/browserterm/client/background-performance.mjs \
  http://127.0.0.1:PORT APP_PID /tmp/long-source-results.json
```

The existing `BenchmarkVideoTrim` now opens source files as streams, separates
validation, and measures first/middle/end preparation and Apply. Use the existing
compiled-benchmark invocation with `THOUGHTS_VIDEO_BENCH_FIXTURE` pointing to the
long fixture. Browser and native harnesses stream fixture hashes/input rather
than loading a multi-gigabyte source into their own memory. Preparation still
loads only bounded derivatives where the established API requires bytes.

Routine CI uses small two-hour synthetic media, virtual large containers and
counting readers for the exact 8 GiB boundary. The real smoke source above was
2.64 GiB; an actual 8 GiB transfer was not measured. Extremely long GOPs, large
indexes, unsupported timestamps/codecs and processing deadlines can still cause
safe rejection. A two-hour, two-FPS single-GOP regression proves that a late cut
cannot expand the unchanged 7,200-frame decode budget. Original sources remain
unencrypted temporary presentation inputs, not archived attachments; abnormal
termination can leave large private staging files. See the media-handling policy
for cleanup and disk-exposure details. Raw qualification artifacts were kept under
`/tmp/thoughts-long-video-review`; fixtures can be regenerated with the recipe.

### Matched short-source comparison

The short fixture uses the same 1080p30 seed, 60 seconds/1,800 frames, SHA-256
`e3bb86156e6f9f930e8e3bf60dcec9c373b93b6dff3f61675b26137c9a6b850e`.
After other worktrees' tests finished, three alternating baseline/current runs
measured the same ten-second late cut. The baseline tree was identical to merged
master `afb7939`; compilation happened before timing. Native preparation medians
were **4.741 s baseline** (4.590–4.800) and **4.667 s current** (4.614–4.723).
The ranges overlap: this workload shows no detectable regression, rather than
an established speedup. Earlier contended, non-alternating results were excluded
from that comparison. Short-source staging medians from the shared-load series
were 146 ms baseline and 137 ms current; those are not controlled speedup claims.
The source copy/probe boundary changed, while the encoder arguments and bounded
prepared-output checks remain the same.

### Four-hour source ceiling

The source ceiling is now four hours, with the same 8 GiB byte limit, 32 MiB
source-index limit, 1–30-second derivative, quality tiers, transfer/processing
deadlines and per-preparation decoded-work budgets. Increasing duration alone
adds no recurring playback work. Transfer cost still depends on source bytes;
index size and keyframe spacing can affect validation and seeking. The measured
2.64 GiB/two-hour results above are not four-hour performance measurements.
Small synthetic four-hour sources verify late native cuts and browser-local
seeking/upload reuse. MKV remains outside this PR: container validation and
cross-browser local preview require a separate compatibility design.
