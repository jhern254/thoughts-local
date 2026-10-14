# Static background budget and performance

Imports retain the existing source limits: non-animated JPEG/PNG, 16 MiB,
8192 pixels per axis, and 24 million decoded pixels. Stored imports are reduced
proportionally to at most 4 million pixels and 3072 pixels per axis, without
upscaling. All new imports re-encode to remove private metadata and normalize
orientation into pixels. JPEG uses quality 85; PNG retains transparency. Existing
assets are unchanged; reimporting applies the budget and sanitization policy.

The durable product policy is documented in [Media handling and privacy](media-handling-policy.md). This document records implementation details, measurements, and limitations for specific revisions; historical benchmark results are not current privacy guarantees.

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
