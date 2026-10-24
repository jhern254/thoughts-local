# Media handling and privacy policy

**Status:** Agreed architectural direction. Image background implementation originated in PR #72; bounded looping video extends the same display-copy policy. General attachments, vault encryption, sync, and external AI disclosure remain future work.

**Decision date:** 2026-10-13

## Decision: policy follows the asset's purpose

Browser backgrounds are presentation assets. Thoughts retains an application-owned, bounded, metadata-minimal display copy. Wallpaper import is not an archival import, and the user's source file is never modified.

Future personal-media attachments follow a different preservation policy: an immutable private original, selectively extracted metadata, and purpose-specific derivatives. Encrypted storage is the target for all retained sensitive representations, not an existing guarantee from PR #72.

The governing rule is:

> Preserve privately when preservation is the purpose; parse selectively; treat embedded values as untrusted claims; disclose explicitly.

A photo attached to an event may retain GPS in its private original while its background or sharing derivative omits GPS. Creating a derivative must not destroy an attachment original.

This decision does not require attachment tables, a generic media service, encryption, or a processing framework in the background PR.

## Separate the responsibilities

**Preservation** decides what information the user retains. Preserving original bytes does not require indexing or interpreting every embedded tag.

**Parser and resource safety** protect the application from malformed media, excessive allocation, unsafe paths, and uncontrolled processing. Removing metadata is not a substitute for safe parsing.

**Authorization and encryption** protect access to stored data. Re-encoding is not encryption; encrypting an original does not make it safe to decode.

**Disclosure** decides what each consumer receives: the browser, a local model, another device, a sync service, an external AI provider, or an export recipient. Importing data does not authorize every downstream use.

## Metadata policy matrix

This is a product policy, not a claim that every listed feature exists. “Preserve” means preserving bytes in the future private attachment original. It does not mean parsing every tag or delivering the original to the browser.

| Information | Future private attachment original | Structured application metadata | Background/display derivative | Default external share or AI input |
| --- | --- | --- | --- | --- |
| Dimensions, duration, codec, sample rate | Preserve | Extract technical facts needed by supported features | Keep/recompute what rendering or playback needs | Include only necessary technical information |
| Orientation/display transform | Preserve | Record only if useful | Normalize into pixels or a supported playback representation | Prefer normalized output without source EXIF |
| Color profile/display information | Preserve | Optional summary, not arbitrary profile text | Preserve compatible display-critical information or use reviewed conversion | Preserve supported appearance without unnecessary identifying fields |
| Capture date, timezone, offset | Preserve | Extract for timeline features; retain uncertainty and source | Omit embedded capture metadata | Omit unless explicitly selected or required for the authorized request |
| GPS, altitude, location | Preserve privately | Extract/index only for an enabled location feature | Omit | Off by default; explicit authorization before disclosure |
| Camera/device/lens model | Preserve privately | Do not extract without a concrete feature | Omit | Omit by default |
| Device serials and owner identifiers | Preserve within the private original | Do not index by default | Omit | Omit by default |
| Captions, keywords, comments, creator text | Preserve | Import useful fields deliberately as untrusted text; separate user corrections | Omit hidden source text; intentional application captions are separate | Include only user-selected content |
| Embedded thumbnails and vendor MakerNotes | Preserve without automatic traversal | Do not extract by default | Generate bounded previews; omit source blobs | Omit |
| Provenance/content-credential data | Preserve with the original | Verification/summary is a separate future feature | Do not claim a derivative is the unmodified original; retain its internal source relationship | Explicit export policy; no automatic source-manifest disclosure |
| Unknown metadata | Preserve opaquely in the original | Do not parse/index by default | Omit | Omit |

The attachment-original column assumes the future protected vault architecture. It does not authorize silently storing unencrypted originals today.

GPS, filenames, capture times, captions, and other extracted metadata are private user data, not low-sensitivity information merely because they are called metadata. Embedded values must never determine ownership, access permissions, storage paths, executable commands, or trusted AI instructions.

Metadata minimization is not anonymization. Pixels may reveal locations, people, documents, or text. This policy does not promise removal of information hidden in pixels or numerical color data.

## Background import contract

The background pipeline is:

    bounded JPEG/PNG source
        -> inspect required display information
        -> decode and validate supported color/profile combination
        -> normalize orientation into pixels
        -> resize without upscaling when needed
        -> encode a new image
        -> add only approved display color information
        -> verify output bounds
        -> store the asset and commit its selection

Every new import is re-encoded, including small images. Application-owned copies are not byte-for-byte archives.

Source limits are 16 MiB, 8,192 pixels per axis, and 24 million decoded pixels. Stored backgrounds are limited to 3,072 pixels per axis and 4 million pixels. JPEG output is lossy at quality 85. PNG retains alpha and supported sample depth; resampling does not promise identical pixels.

After orientation is applied, the output omits source EXIF/eXIf, location/device/date metadata, XMP, IPTC, comments, embedded thumbnails, and unnecessary source text. Only supported display color declarations and the restricted sanitized ICC representation are retained.

The profile path is a restricted sanitizer, not a general color-conversion engine. It supports selected compatible RGB/grayscale matrix/TRC profiles. CMYK, unsupported LUT/HDR declarations, incompatible profiles, and profiled grayscale-plus-alpha PNG are outside the supported path. Acceptance is not guaranteed for every otherwise-valid JPEG or PNG.

Do not expand custom profile support casually or strip a required profile silently to accept an image. Broader support should trigger an explicit evaluation of a maintained processor, not incremental construction of a custom color-management engine.

Existing assets are not rewritten at startup. Explicit replacement/reimport applies the current policy. Older assets, orphaned files, and backups may still contain source metadata. Local background assets are not encrypted yet; the future vault must include them.

Framing changes preserve the imported display copy. The source file on the user's device remains untouched. Wallpaper import does not archive an original inside Thoughts.

## Looping video backgrounds

Video backgrounds are presentation assets with the same no-archive policy.
Thoughts stores one prepared silent H.264 display copy and a generated JPEG
poster; the original source on the user's device is untouched. Audio, subtitles,
data streams, chapters, source location/device/date metadata and comments are
not retained. Display rotation is normalized into frames; only supported SDR
color interpretation and necessary playback/container information remain.

### Upload compatibility at a glance

An `.mp4` or `.mov` extension describes the container, not the video encoding
inside it. Supported uploads must meet all applicable limits below and be
decodable by the browser used for source preview.

| Video characteristic | Current support |
| --- | --- |
| H.264, 8-bit SDR with supported Rec.709 color | Supported; untagged color is interpreted as Rec.709. |
| Standard or self-contained fragmented MP4 | Supported; fragmentation alone does not require re-exporting the source. |
| Nonzero starting timestamps | Supported; some late-frame previews are limited by the browser. Native Preview loop remains available. |
| Large or long sources | Up to 8 GiB and 1 second–4 hours. Saved loops remain 1–30 seconds. |
| 1080p and 4K | Supported within 4,096 pixels per axis and 9 million pixels per frame. |
| Frame rates up to 60 FPS, including 29.97/59.94 | Supported. |
| Portrait video and ordinary 90° rotation | Supported; orientation is normalized in the saved loop. |
| Audio/subtitle tracks | Allowed, then removed; one video track and four total tracks maximum. |
| HEVC/H.265, AV1, VP9 or ProRes inside MP4/MOV | Not currently supported. |
| HDR, 10-bit video or explicitly tagged colors other than Rec.709 | Not currently supported, including some valid older SDR files. |
| 120 FPS, 8K or sources exceeding the limits above | Rejected at selection. |
| Encrypted video, external-file references, malformed or excessively complex containers | Rejected for safety or bounded processing. |

The main format gaps are HEVC, HDR/10-bit and other color formats. These are
implementation limitations, not evidence that such files are unsafe.
Selection checks catch known incompatibilities; damaged frames, unusually
expensive decoding or storage failures can still prevent a particular preview.
Preparation failures leave the existing background selected and report fixed
feedback.

### Prepared output

Import quality is a persisted preference, initially Efficient, with these output
ceilings:

| Quality | Longest axis | Pixels/frame | FPS | Video bytes | VBV maximum rate / buffer |
| --- | ---: | ---: | ---: | ---: | ---: |
| Efficient | 1280 | 921,600 | 30 | 16 MiB | 3 Mbps / 6 Mbit |
| High quality | 1920 | 2,073,600 | 30 | 32 MiB | 6 Mbps / 12 Mbit |
| Maximum | 1920 | 2,073,600 | 60 | 64 MiB | 10 Mbps / 20 Mbit |

Portrait footage uses the same dimension/pixel budget. Output uses even
dimensions and square pixels, preserves display aspect ratio after supported
rotation/sample-aspect correction, and never enlarges source samples. Frame
rates remain exact rationals, capped only when above the selected tier's ceiling.
No interpolation is performed. H.264 Constrained Baseline, one reference frame,
no B frames, CRF 23 and bounded VBV are retained; a still scene need not fill its
bitrate budget. Level 3.1, 3.2, 4.0 or 4.2 is selected from padded 16×16
macroblocks/frame, macroblocks/second, reference-picture, axis and bitrate/buffer
bounds, following the H.264 Annex A limits also used by
[FFmpeg's level selection](https://github.com/FFmpeg/FFmpeg/blob/n6.1/libavcodec/h264_levels.c).
Visible even dimensions alone do not establish level compatibility.
Re-encoding is lossy. Untagged SDR
is interpreted as Rec.709; explicit incompatible primaries/transfer, ICC video
profiles, HDR/HEVC, encrypted/reference movies, arbitrary display
transforms and unsupported encodings are rejected rather than silently relabeled.

Native processing stages private application-named files, uses explicit argument
arrays and a local-file protocol allowlist, validates self-contained container
references, limits work and joins canceled subprocesses before cleanup. FFmpeg
and ffprobe are installed separately and must be maintained by the user. This
boundary is not an OS sandbox; protocol restrictions alone do not confine files.
Staging bounds the source bytes/container/stream declarations without rejecting
a four-hour source merely because its full duration exceeds a decoded
pixel budget. Each preparation instead counts decoded frames before timestamp
trimming and FPS reduction, including preceding-keyframe seek preroll. Exceeding
four billion nominal source pixels or 7,200 frames cancels and joins FFmpeg.
Decoder and pipe read-ahead can occur; this is an application work budget, not
a hard CPU, process-memory or OS sandbox limit. Extremely long GOPs can make
a late cut exceed it; choose an earlier/shorter cut or export a source with more
frequent keyframes. Transfer has a finite 20-minute total limit and a 30-second no-progress read
limit. Native container/probe validation receives a fresh two-minute deadline
after transfer; each preparation retains its separate two-minute deadline.
Increasing source duration does not increase the decoded-frame or pixel budgets.
Self-contained fragmented MP4 is accepted directly: its packet indexes are
spread across fragments rather than stored in one initial movie index. FFmpeg
reads that layout without a preliminary whole-file remux. Initial movie and
fragment metadata share the 32 MiB source-index budget; prepared output remains
nonfragmented. External file references and encrypted movies remain rejected.
Source movie indexes are bounded to 32 MiB; prepared-loop indexes remain bounded
to 8 MiB. Four-stream, nesting, box-count and processor-allocation limits remain.

Prepared output has its own tier-size limit, container/stream checks, actual
packet cadence/count and VBV-envelope validation, and a bounded full independent
decode before selection is committed. Poster geometry uses the same output plan.
Decoder allocation permits small SIMD padding around the stricter visible-pixel
limits; padding does not permit larger stored dimensions. Frame-aware duration
checks allow fractional-FPS rounding while retaining a 1–30-second prepared loop.

Speed and deliberate pause persist. Visibility, connection, reduced-motion,
preview exclusivity and playback failure are browser lifecycle conditions, never
saved pause preferences. A stable poster covers loading, buffering, pause and
failure. No frames travel through the terminal or WebSocket. Local video/poster
assets and staging files are not encrypted; crashes can leave unselected files.
Future vault work must include these assets.

Choosing a video streams it once into private native staging and checks its
container and technical properties before opening the cut editor. Known
unsupported codecs, color formats, dimensions, frame rates and durations receive
fixed rejection messages at selection. This inspection does not encode or decode
the complete movie. It is not a guarantee that every later cut will succeed:
damaged frames, processing budgets, disk failures and cancellation can still
prevent preparation. A prepared loop is usable only after output validation and
browser acknowledgement.

The browser keeps the File/object URL and one muted, paused source-video element.
Moving Start or End seeks that element without another upload, native processing or
thumbnail generation. Previews reuse the validated native source. The application
never buffers the whole movie into a JavaScript ArrayBuffer or Go byte slice.
Nonzero timestamp origins are accepted; source-relative controls account for the
browser's origin. Where Chromium cannot seek a late source frame, a fixed notice
directs the user to native Preview loop instead.
New-upload trimming temporarily retains that source while Visual is open; Apply, Cancel, closing
Visual, disconnect and orderly shutdown remove it after joining processing.
It is not an archive; saved clips have no retained source or re-trim metadata.
A crash can leave staging files, as with other incomplete filesystem operations.
An 8 GiB source can occupy roughly that much additional temporary disk space,
plus bounded candidates and accepted output. Sources and derivatives are
unencrypted; larger staging increases the amount and duration of private data
exposed on local storage. Orderly cleanup is not secure erasure. Unsupported,
poorly indexed or very long-GOP movies may still be rejected. The source file
on the user's device is never changed.
Only explicit Preview loop encodes; Apply saves the last acknowledged prepared
video and poster unchanged. Draft cut or quality edits do not replace that
accepted loop. Its quality, dimensions, rational FPS and interval belong to the
accepted bytes. Apply saves its quality preference too; settings-only quality
changes affect future imports without changing the selected file. Playback
speed, framing and darkness never encode video.
At 25% speed, a 30-second clip plays for 120 seconds without storing more frames.
No attachment originals are introduced by this feature.

## Storage, processing, and replacement safety

Use generated identifiers, controlled asset routes, bounded input and processing, appropriate file permissions, and the established local Host/Origin checks. Do not expose a general filesystem directory or log image bytes, original filenames, private metadata, paths, or raw decoder errors.

Validate and successfully write a replacement before committing its selection. Failure before commit preserves the old selection. Failure to delete the previous file after a successful commit is a cleanup warning, not a failed save.

Filesystem operations and SQLite are not one atomic transaction. Document crash leftovers and reconciliation; do not describe sequential writes as an atomic cross-store commit.

Keep cancellation ownership explicit around bounded non-preemptible decoder/scaler/encoder calls. Do not introduce detached work to simulate cancellation. Richer video/audio processing requires a separate resource and isolation review. A worker process alone is not a sandbox.

## Future attachments: originals, facts, and derivatives

The target representation is:

    private attachment
        + immutable encrypted original
        + selectively extracted encrypted metadata
        + bounded encrypted derivatives, where retained

Keep embedded source values separate from user corrections and inferred values. EXIF location, user-entered location, and AI-estimated location must remain distinguishable. Missing timezones or uncertain dates must not be silently made precise.

Normal presentation uses a supported derivative. Original export is explicit. Unprocessed or rejected input must not be marked display-ready. Whether unsupported originals may be archived without rendering is a separate future decision.

Using an attachment as wallpaper creates a background derivative while retaining the attachment original. Importing wallpaper alone must not silently create a personal-media archive or populate timeline/location records.

Reuse suitable transformation and storage-safety helpers only when concrete attachment features need them. Do not use the background `prepareImage` pipeline as the preservation path for canonical attachment originals.

## External disclosure, backup, sync, and deletion

A private backup may contain encrypted originals, useful metadata, derivatives, and their relationships. Share and external-AI copies default to bounded, metadata-minimal derivatives. Include location or other contextual fields only for an explicitly authorized purpose. Local AI should also receive only task-relevant data.

Local import is not consent for remote AI processing. Original export must explain that embedded metadata is retained.

For future server-blind sync, authorized devices encrypt content and sensitive metadata before transmission. Minimize and document visible routing/version/size metadata. A global plaintext content hash can disclose equality across users; cross-user deduplication is not an incidental implementation choice.

Deletion, backup expiration, device revocation, recovery, and provenance verification need their own designs. Removing an app asset does not erase the user's source file or every historical backup.

## Testing and maintenance

Keep deterministic tests for limits, orientation 1–8, supported color behavior, unnecessary-tag removal, alpha/sample depth, failed replacement, restart persistence, and browser ownership/focus. Inspect containers/tags as well as synthetic private markers; string searching alone is not a complete privacy test.

An existing `FuzzDisplayMetadata` target exercises orientation, JPEG/PNG metadata, and profile handling. Reuse it and improve meaningful seeds where necessary. Ordinary tests run the seed corpus; active fuzzing is separate evidence. Fuzzing and passing tests are not a security certification.

The intermittent black-rectangle report remains unresolved. Readiness and one-shot repaint safeguards do not establish its root cause. Do not claim a compositor fix, universal color support, or zero privacy leakage.

## Scope boundary for backgrounds

No attachment schema, original-media archive, GPS indexing, encryption/key management, sync, cloud AI, general video processing, premium system, generic metadata framework, or new color-conversion engine belongs in this finalization.

Finish the bounded background importer. Record the future attachment architecture without implementing it prematurely.

## Related documents

- [Browser bridge maintenance](../internal/browserterm/UPSTREAM.md)
- [Background performance and implementation evidence](browser-background-performance.md)
- [Background preparation](../internal/visual/image.go)
- [Metadata tests and fuzz target](../internal/visual/metadata_test.go)
- [Restricted profile handling](../internal/visual/color_profile.go)
