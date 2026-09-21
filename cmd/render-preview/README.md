# Distribution preview

The default is the app-style `main` scene, with cards, full event times, and the
Now marker. Use `distributions` as an additional comparison of curve overlap.
Review both scenarios when changing the renderer.

Run from the repository root:

```sh
go run ./cmd/render-preview # same as -scenario main
go run ./cmd/render-preview -scenario distributions -selected 2
go run ./cmd/render-preview -mode outline
go run ./cmd/render-preview -scenario scale -count-exponent 0.25 -size 1
go run ./cmd/render-preview -width 60 -height 28
go run ./cmd/render-preview -scenario crowded -selected 6
go run ./cmd/render-preview -plain > /tmp/distribution-preview.txt
```

This is a **static synthetic preview**, not the live Events screen. The footer
shows the application's UI composition; its keybindings do not operate here.
No database is opened by this command; it does not exercise Events loading or navigation.
The actual new renderer supplies the distributions; the surrounding cards use
fixed fixture rows matching the accepted design. Narrow/short previews truncate
text and crop earlier rows to retain the ending marker, rather than recalculate
time geometry. An outline can therefore be partially outside the viewport.

Options:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-mode` | `filled` | `filled` or `outline`; both follow the same Gaussian profile |
| `-scenario` | `main` | `distributions`, `main`, `adjacent`, `gapped`, `crowded`, `scale`, or `empty` |
| `-count-exponent` | `0.8` | Positive exponent: lower values boost smaller counts; try `0.25` |
| `-size` | `1` | Scale gains from `0.5` to `1.25`, within the fixed lane |
| `-selected` | `3` | One-based event number; `0` or an absent event means no highlight |
| `-width` | `100` | Terminal columns, at least 32 |
| `-height` | `36` | Terminal rows, at least 12 |
| `-plain` | `false` | Omit ANSI colors for text captures |

The main scene has 20, 0, and 10 thoughts. The crowded scene includes counts above
20 to demonstrate day-relative scaling. The `scale` scene compares 0, 20, 100,
and 200 thoughts with full event cards. Historical scenarios retain `...`; today's scenes
end at Now. The selected curve and border use ANSI color 62, unselected curves
use gray 245, and the connecting baseline uses gray 240. Both modes share the same
selection behavior. Color choices are in this preview, not the renderer.

The `distributions` scenario compares three renderer-only profiles with the same
counts (20, 20, and 0) at different distances. It shows separated peaks, shared
valleys where tails overlap, and close peaks merging into a broad mound. It has
no event cards or timestamps: it isolates curve behavior from Events placement.

## Live Events integration

The Events screen uses the same renderer for collapsed cards with known counts.
It reserves this separate lane at panel widths of 60 columns and above; below
60, it hides the lane and restores the original card width. Expanding an event
removes its contribution while retaining other collapsed curves. Unknown or failed
counts do not draw a mound; a known zero count does. Existing successful counts
remain visible during refresh when their event intervals are unchanged.

Press **d** in Events to open curve controls in the existing footer rows:

| Control | Effect |
| --- | --- |
| Left / right | Less / more boost for smaller counts |
| Down / up | Smaller / larger curves, from 50% to 125% |
| **f** | Toggle filled / outline |
| **0 — Reset to default** | Restore normal boost, 100% size, and filled mode |
| Enter / Esc / d | Close controls, keeping changes |

The presets and reset values are defined in
[`internal/tui/events/distribution.go`](../../internal/tui/events/distribution.go).
The default boost uses exponent 0.8. Medium uses 0.5, strong uses 0.25, extra
uses 0.15, and linear uses 1. With a daily maximum of 200, **strong** makes the
20-thought curve approximately as large as the old normal 100-thought curve.
Size multiplies the width and height gains without changing the zero mound.
Neither control allocates card rows or widens the 10-column lane.

Changes apply immediately and persist while navigating days, switching panels,
and leaving/returning to Events in the same process. They are not saved across
application restarts. Tab closes the controls and changes panel focus normally.
Below 60 columns the curves remain hidden, with a notice in the control panel.
Focus controls purple highlighting; changing fill mode preserves that behavior.

This static preview keeps its fixed fixture layout; use the actual seeded TUI to
review responsive widths and interactions:

```sh
make tui/demo/seeded
```

## Renderer API and tuning

```go
cells := render.RenderDistributions(10, 24, []render.Distribution{
    {CenterY: 31.5, Count: 20},
}, render.Options{}) // Filled by default; set Mode: render.Outline for outlines.
```

Dimensions are terminal cells. `CenterY` is a vertical **Braille dot coordinate**;
each cell has two horizontal dots and four vertical dots. For a card beginning at
row `top` with height `h`, use `4*top + (4*h-1)/2`, evaluated with floating-point
division. The renderer returns space/Braille glyphs and a `CurveIndex` for styling.
An index of -1 identifies blank cells or the connecting baseline. A cell can have
only one foreground color, so its strongest visible contribution determines its
owner; exact ties favor the earlier input.

The equation and tuning constants are documented together in
[`internal/render/distribution.go`](../../internal/render/distribution.go).
Counts and size control both the leftward amplitude and vertical span of an unnormalized
Gaussian. They do not represent a probability density, event duration, or a fit
to the timing of individual thoughts.

- Set `Options.CountExponent` (default 0.8): raise toward 1 for less boosting,
  lower toward 0 for more. Counts remain ordered, but neighboring values can
  share Braille pixels; very small exponents reduce visual differences.
- Set `Options.Size` (default 1): scales the gains within `[0.5, 1.25]`.
  Invalid/nonpositive values use the default; finite positive values are bounded.
- Adjust `minAmplitude` and `minHeight` to tune the shallow zero-thought mound.
- Adjust the amplitude/height gains to change the maximum dimensions.
- `countReference` is a minimum reference of 20, not a cutoff. The actual
  reference is `max(20, Options.CountReference, all valid input counts)`.
  The Events caller includes the full day's known counts, even expanded cards.
  Scrolling and hiding an expanded curve cannot reset this count reference.
  Counts compare within a day; the same count can look different on another day.
- `tailSigma` controls the retained tail extent; `samplesPerDot` controls sampling.

The renderer adds the Gaussian contributions into one continuous profile:

```text
S(y) = sum(A_i * exp(-0.5 * ((y-center_i)/sigma_i)^2))
gain = min(1, maximumDisplayAmplitude / max_y(S(y)))
x(y) = baseline - gain*S(y)
```

At default size the maximum display amplitude is 16 dots; size scales its gain,
and the result is always bounded by the available lane width. If the sum
exceeds it, one gain scales the entire profile horizontally, preserving valleys
and relative peak heights instead of clipping a flat top. This gain uses all
supplied distributions, including offscreen ones, so scrolling cannot change the
shape. Isolated distributions retain their count-relative scale. Removing a contribution
can still change the summed shape or its fitting gain; it does not reset the
count reference. Overlapping peaks can
remain distinct or merge, depending on their distance and spread; the profile is
not forced to have one peak per input. Color follows the strongest contributing
kernel, independently of the summed geometry. A zero-thought mound is decorative,
not evidence of measured activity.

Filling shades from the sampled boundary to the baseline. Outline traces the
curve at dot-row centers and joins consecutive rows where the curve crosses
their shared half-row. Each join shares a dot column: diagonal-only gaps and
backward-facing stubs interrupt the stroke. This centerline can differ from the
filled edge by one Braille dot while preserving the Gaussian shape and position.
Neither mode allocates card space or shifts timestamps.
Colors and layout remain caller responsibilities. The preview leaves two-column
gutters around the ten-column curve lane and excludes the ending marker from the
drawing bounds. Live Events keeps those same responsibilities.

## Verification

```sh
go test ./internal/render ./cmd/render-preview -count=1
make ci
```

The checked-in plain-text snapshots preserve whitespace and were reviewed at
100x36 (both modes) and 60x28 (filled). Color assertions are separate. For visual
review, run both modes in terminals at those sizes and inspect the zero mound,
connected tails, selected/unselected colors, card boundaries, and ending marker.
Keep transient captures under `/tmp`; do not regenerate snapshots merely to make
a failing test pass. The static preview does not validate live panel switching,
day navigation, expansion, or asynchronous ownership.
