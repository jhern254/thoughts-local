# Browser background architecture and performance

Background processing spends bounded CPU at import so normal use can reuse a
small display asset. Static images are the lowest-cost option; looping video
adds recurring browser work even when native decoding is available.

This page summarizes the architecture and representative measurements. The
[media-handling policy](media-handling-policy.md) defines privacy, compatibility,
and resource limits; [browser bridge maintenance](../internal/browserterm/UPSTREAM.md)
explains ownership and integration. Measurements below belong to the named
revisions, not automatically to the latest build or its privacy guarantees.

## Architecture

- **Go owns import and persistence.** Validate private staged inputs, prepare a
  bounded derivative, independently validate it, then commit its selection.
  Failed replacement keeps the working background. Sources are not archived.
- **Bubble Tea owns controls.** Options exposes framing, darkness, quality, and
  playback preferences without sending media frames through the WebSocket.
- **The browser owns display.** Native image/video elements sit behind the
  readability overlay and transparent terminal. Settings-only changes reuse
  the active media instead of decoding or loading it again.
- **Work follows its owner.** Cancellation and generation checks retire obsolete
  imports, previews, and connections. Preview playback pauses the selected
  background; hidden/disconnected pages and reduced motion stop automatic
  playback. A poster provides a stable fallback.

### Import privacy and color contract

Every new JPEG/PNG import is decoded and re-encoded, including small images.
Orientation is applied to pixels; unnecessary source metadata is discarded.
Supported display color information is retained through the restricted profile
sanitizer. Unsupported color encodings are rejected rather than silently
misinterpreted. Existing assets are not automatically rewritten.

Image sources are limited to 16 MiB, 8,192 pixels per axis, and 24 million pixels.
Stored copies fit within 3,072 pixels per axis and 4 million pixels, without
upscaling. JPEG output is lossy; PNG retains transparency and supported sample
depth. These limits bound stored assets, not all transient decoding memory.

### Video preparation and preview

Compatible MP4/MOV and Matroska sources are staged once, with an 8 GiB/four-hour
ceiling. Only the chosen 1–30-second loop is encoded. Apply copies the exact
acknowledged video/poster bytes, without a second encode. Source-relative cuts
use verified timing; long sources do not increase the per-cut decode budget.

Browser-local seeking is preferred when the selected file and timeline work.
Otherwise, native still-frame extraction provides a bounded fallback. Rapid
requests are coalesced, with at most one extraction active per editor; stale
frames cannot replace newer results. There is no full-movie conversion or
permanent preview proxy.

FFmpeg runs with explicit local inputs, selected streams, finite deadlines and
resource budgets. Prepared output is silent H.264 MP4 with a JPEG poster.
Subprocess restrictions are not an OS sandbox or a hard total-memory ceiling.
Temporary sources are unencrypted, and abnormal termination can leave staging
files. See the policy for the full compatibility and cleanup contract.

## Benchmark context

The reference machine is a ThinkPad T480s, Intel i5-8250U/UHD 620, eight logical
CPUs, Linux amd64, Go 1.25.14, and FFmpeg 6.1.1. Browser studies used Brave
148.0.7778.217; Chromium and Firefox also received functional acceptance checks.
Most stress runs used 721 Events and 20,000 Thoughts.

CPU percentages mean **one logical CPU**, not the entire machine. Native and
browser costs are measured separately; Go allocations exclude FFmpeg. Browser
RSS can double-count shared pages, so PSS is preferred for comparisons.
Navigation timing measures matching terminal updates, not physical display
latency. Small sample counts, warm caches, picker handoff, and shared-machine
variance limit conclusions. Failed runs were excluded from successful medians,
not treated as successes. These are previous measurements; this documentation
cleanup introduces no new benchmark results.

### Static image import

The original stored-image budget comparison (`f34d0b9` → `51df113`) reduced
6000×4000 browser activation from **122.2 to 34.6 ms**, while native import rose
from **330 to 603 ms**. That is a one-time processing tradeoff, not faster
navigation. This comparison predates sanitizing every small import.

The privacy follow-up compared `ca088c9` with the sanitization implementation
using five-run native medians:

| PNG source | Native import before → sanitized | Go allocation before → sanitized |
| --- | ---: | ---: |
| 1267×719 | 13.0 → 40.9 ms | 3.71 → 5.00 MB |
| 6000×4000 | 602.0 → 604.8 ms | 114.65 → 114.65 MB |

Small imports gained about 28 ms of one-time work because original bytes are no
longer retained. Large imports already resized and encoded. Browser Apply through
return/selection rose from 462 to 489 ms for a normal image, while large-image
activation remained about 29 ms. Picker timing varied substantially; these runs
do not establish a preview speedup. Earlier static idle snapshots measured about
1% browser CPU and 2% native CPU, with unequal before/after sampling.

### Video quality through 1080p60

Efficient remains the default. Quality ceilings are not upscaling targets;
lower source frame rates remain lower. Playback speed changes do not re-encode.

The quality study at `09d8302` used two matched 30-second 1080p60 fixtures:
synthetic motion and public-domain [NPS ambient footage](https://www.nps.gov/media/video/view.htm?id=39E50741-155D-451F-67CACF2694A2F3E7).
The ambient benchmark repeated original 23.976 FPS frames; it was not natural
60 FPS capture. Values span the two fixtures, using three-run medians except
for the previously completed five-run motion/Efficient navigation series.

| Tier | Stored ceiling | Native preparation | Browser CPU, full speed | Browser CPU, half speed |
| --- | --- | ---: | ---: | ---: |
| Efficient | 720p30, 16 MiB | 21–25 s | 24–26% | 15–16% |
| High quality | 1080p30, 32 MiB | 28–36 s | 27–29% | 15–16% |
| Maximum | 1080p60, 64 MiB | 43–50 s | 42–51% | 26% |

FFmpeg sampled peak RSS was about 128–193 MiB across tiers. Paused and
reduced-motion browser CPU settled around 0.7–1.5%, with no advancing frames.
Synthetic microphone capture raised browser CPU to roughly 40–58%; this
**excludes speech inference**. Quarter-speed playback in the earlier 24 FPS
study reduced CPU but visibly stepped motion; it adds no interpolated frames.

The following retained detail separates motion complexity from quality settings.
Playback CPU/PSS values are medians of three eight-second headed-browser samples;
native preparation is the median of three one-iteration runs. Navigation used
1200×850, with a separate 650×600 walkthrough; these are not full-HD viewport
composition measurements.

| Fixture / tier | Prepare, s | FFmpeg peak RSS, MiB | Full-speed browser CPU | Browser PSS, MiB | Native app CPU |
| --- | ---: | ---: | ---: | ---: | ---: |
| Motion / Efficient | 21.04 | 128.18 | 23.8% | 291.8 | 2.0% |
| Motion / High quality | 28.27 | 187.90 | 26.9% | 379.2 | 2.2% |
| Motion / Maximum | 43.12 | 190.70 | 41.6% | 344.4 | 1.9% |
| Ambient / Efficient | 24.81 | 130.80 | 26.2% | 359.0 | 2.1% |
| Ambient / High quality | 36.38 | 190.54 | 28.8% | 391.5 | 2.1% |
| Ambient / Maximum | 49.58 | 193.11 | 51.3% | 348.9 | 2.1% |

Native app CPU excludes FFmpeg. Peak FFmpeg RSS is sampled across sequential
phases, not combined app/child memory or a hard ceiling. PSS includes browser
cache/state differences; it does not establish a memory ranking among tiers.
Normal-speed steady samples reported zero dropped frames; playing-navigation
counters recorded 3–16 drops over roughly 3,000–6,000 frames, including startup
and settling. Passing playback does not mean perfect frame delivery.

| Playing navigation, motion fixture | Efficient median/p95, ms | Maximum median/p95, ms |
| --- | ---: | ---: |
| Event selection | 15.0 / 30.1 | 19.9 / 30.7 |
| Expand/collapse | 18.6 / 34.4 | 23.2 / 32.9 |
| Traverse 200 Thoughts | 17.5 / 31.2 | 22.7 / 30.5 |
| Day changes | 166.8 / 185.5 | 174.3 / 195.8 |
| Typing | 14.0 / 14.8 | 16.1 / 30.1 |

Each latency column is the median of per-run medians/p95s, not a pooled percentile.
The workload traverses the day's Events in both directions and all 200 Thoughts
in its ongoing Event; typing uses three eleven-character sequences.

The initial target of less than 10% additional playback CPU was **not met**.
Maximum-quality day-navigation p95 reached 196–211 ms versus about 174–175 ms
for its static controls, also exceeding the initial +10 ms target in that study.
These historical day-navigation numbers are not a benchmark of later navigation
fixes. Prefer Efficient, slower playback, pause, or a static image when reserving
CPU for other work.

Brave reported platform decoding on the reference machine; this does not prove
all work ran on the GPU. Firefox played the ambient 1080p60 loop with zero
reported drops in a short check, but its CPU/hardware path was not measured.
macOS/Windows runtime performance remains unverified.

### Trimming and Matroska

At `0b70c98` versus `357737d`, the trimming pipeline reduced Go allocation for a
30-second preparation from **73.2 to 21.0 MB**. Apply reused prepared bytes;
a separate two-minute-source test measured about **26 ms native Apply**, versus
**3.4–3.8 s** to prepare a ten-second loop. Browser activation/return adds work
beyond that native copy.

The MKV study compared master `4a329a3` with processor `6e1e092`, followed by
`9eca7b7` selected-video-origin validation. The added bounded timing probe costs
roughly another 80–110 ms per MKV validation; it is not repeated per nudge.

| Native operation | Two-hour H.264 MKV | HEVC Main10 SDR MKV |
| --- | ---: | ---: |
| Stage, including validation (`9eca7b7`) | 10.68 s | 158 ms |
| Validate alone (`9eca7b7`) | 219 ms | 153 ms |
| Prepare ten-second loop (`6e1e092`) | 4.5–4.6 s | 5.9–6.3 s |
| Extract source still (`6e1e092`) | 120–221 ms | 177–595 ms |
| Apply acknowledged bytes (`6e1e092`) | 29 ms | 23 ms |

The two-hour source was 2.64 GiB; the Main10 fixture was 30 seconds of 1080p24
Rec.709. Samples numbered three, with large-copy staging ranging from 4.22 to
10.74 seconds. These are not cold-disk guarantees. Matched short MP4 preparation
stayed around 4.5–4.6 seconds; no material preparation regression was demonstrated.

Browser-local seeks were typically 120–180 ms. Native fallback commonly took
about 0.9 seconds end-to-end, including coalescing, HTTP, and image decode;
one Main10 seek took 1.89 seconds. Final short/two-hour MKV series uploaded once,
issued five fallback requests, and left staging empty after Apply. Native stills
run only during source browsing, not recurring background playback.

#### Matroska end-to-end comparison

The short fixture is 60 seconds of 1920×1080 H.264/30 FPS, 23,627,561 bytes;
MKV remuxes the same packets. The two-hour MKV is 2,835,940,293 bytes with
two-second keyframes. Main10 is a 30-second 1080p24 Rec.709 HEVC source.
Native short-source studies used five samples; large/Main10 used three.

The browser series used three successful repetitions at 1920×1080. Each
repetition includes selection/upload, three direct-timecode seeks, twenty rapid
arrows, a ten-second late cut, acknowledgement, and Apply. All values below are
median milliseconds; native fallback includes dispatch, 150 ms coalescing,
HTTP, and JPEG decode.

| Revision / source / preview path | Selection | Three seeks | Twenty arrows | Prepare | Acknowledge | Apply |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| `4a329a3` / short MP4 / browser | 269 | 124 / 130 / 125 | 401 | 5,265 | 95 | 345 |
| `f2f3b9b` / short MP4 / browser | 267 | 129 / 127 / 377 | 331 | 5,226 | 90 | 402 |
| `9eca7b7` / short MKV / native | 363 | 904 / 896 / 907 | 612 | 5,940 | 190 | 385 |
| `6e1e092` / Main10 MKV / native | 192 | 888 / 1,889 / 882 | 1,112 | 7,240 | 87 | 359 |
| `4a329a3` / two-hour MP4 / browser | 13,651 | 131 / 120 / 120 | 338 | 5,201 | 119 | 423 |
| `9eca7b7` / two-hour MKV / native | 15,397 | 881 / 897 / 377 | 600 | 5,097 | 204 | 399 |

These rows retain their original revisions: they do not all measure the later
selected-video-origin check. Container and preview-path differences prevent
interpreting the long-source rows as a matched encoding speedup.

| MKV workflow | Browser / Go / processor CPU, seconds | Browser peak PSS, MiB | Go / processor peak RSS, MiB |
| --- | --- | ---: | --- |
| Short, `9eca7b7` | 3.04 / 1.58 / 6.55 | 392 | 50 / 112 |
| Main10, `6e1e092` | 2.58 / 1.34 / 9.16 | 411 | 44 / 155 |
| Two hours, `9eca7b7` | 13.49 / 8.37 / 5.34 | 382 | 49 / 113 |

These are one-time workflow costs, not recurring playback. The final two-hour
series transferred in 14.88 seconds, then took 460 ms for validation/response.
Peak staging was 2,838,083,552 bytes, roughly source plus loop, and emptied on
Apply. Earlier intermittent stale-display runs were excluded from successful
medians and remain an unresolved observation.

### Video preparation activity wave

The loading wave renders fixed braille text and animates clipped layers with
CSS transforms. It adds no animation-driven Go ticks or WebSocket messages.
It stops when hidden, canceled, disconnected, complete, or under reduced motion.

A T480s held-request comparison on 2026-10-21 measured **12.6–13.4% browser CPU**,
down from **22.5–23.4%** for repeated text rewrites; the static control was 0.8%.
Those are whole-browser samples, not an isolated animation cost. This remains
measurable work limited to pending preparation, not a free or permanent effect.

## Repeating measurements

Use synthetic media and a disposable migrated stress demo: the browser probe
imports and removes backgrounds. Install its existing client dependencies with
`npm ci` in `internal/browserterm/client`. Keep fixture generation and compilation
outside timing, with no competing tests/builds.

```sh
go test ./internal/visual -run '^$' -bench '^BenchmarkPrepareImage$' -benchmem

THOUGHTS_VIDEO_BENCH_FIXTURE=/tmp/synthetic.mp4 \
  go test ./internal/visual -run '^$' -bench '^BenchmarkVideoTrim$' \
  -benchmem -count=3 -benchtime=1x

THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1 \
THOUGHTS_PERF_REVISION=$(git rev-parse HEAD) \
THOUGHTS_PERF_VIDEO_FIXTURE=/tmp/synthetic.mp4 \
  node internal/browserterm/client/background-performance.mjs \
  http://127.0.0.1:PORT APP_PID /tmp/background-results.json
```

Use `scripts/video-quality-fixtures.sh` for repeatable quality fixtures.
The browser probe supports interaction/quality studies and forced native-frame
fallback; its configuration lives alongside the harness. Record revision,
fixture, browser, viewport, sample count, CPU, memory, and latency together.
Compile Go benchmarks separately when collecting process/child CPU and RSS.
For example, the quality study can be repeated with:

```sh
sh scripts/video-quality-fixtures.sh /tmp/quality-fixtures
go test -c -o /tmp/visual-bench ./internal/visual
THOUGHTS_VIDEO_BENCH_FIXTURE=/tmp/quality-fixtures/motion60.mp4 \
THOUGHTS_VIDEO_BENCH_QUALITY=maximum \
  /usr/bin/time -v /tmp/visual-bench -test.run '^$' \
  -test.bench '^BenchmarkPrepareVideo$' -test.benchmem -test.count=3 -test.benchtime=1x
```

For browser quality comparisons, use `THOUGHTS_PERF_INTERACTIONS=1`,
`THOUGHTS_PERF_QUALITY_STUDY=1`, `THOUGHTS_PERF_VIDEO_QUALITY=maximum`, and
`THOUGHTS_PERF_SAMPLES=3` with the browser command above. Repeat each tier and
fixture. `THOUGHTS_PERF_NATIVE_FRAMES=1` forces the native still-preview path for
its separate comparison. Keep JSON records and fixture hashes with each revision
rather than relabeling old measurements as evidence for new code.

## Remaining limitations

The intermittent black rectangles and occasional stale Visual loading text
remain unresolved. Later successful repetitions do not establish a compositor
fix. Native preview is slower than browser seeking, and long GOPs or pathological
indexes may exceed bounded seek/decode limits. Actual 8 GiB/four-hour transfers
were not performance-qualified; boundary tests are not equivalent evidence.
Historical temporary captures may no longer exist after restarts; tracked
harnesses and fixture recipes allow fresh qualification.
