package visual

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
)

const (
	// Bound compressed input and decoded memory independently.
	MaxImageBytes = 16 << 20
	MaxDimension  = 8192
	MaxPixels     = 24_000_000

	// Stored backgrounds target lightweight 1440p use; source limits still bound decoding.
	MaxBackgroundDimension = 3072
	MaxBackgroundPixels    = 4_000_000
	backgroundJPEGQuality  = 85 // One-time encoding balances detail and stored bytes.
)

var ErrImage = errors.New("choose a valid, non-animated JPEG or PNG within the image limits")
var ErrDarkness = errors.New("background darkness must be between 0 and 95")
var ErrBusy = errors.New("an visual change is already in progress")

func prepareImage(ctx context.Context, body []byte) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if len(body) > MaxImageBytes {
		return nil, "", ErrImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 ||
		config.Width > MaxDimension || config.Height > MaxDimension || int64(config.Width)*int64(config.Height) > MaxPixels {
		return nil, "", ErrImage
	}
	metadata, err := inspectDisplayMetadata(body, format)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	decoded, actual, err := image.Decode(bytes.NewReader(body))
	if err != nil || actual != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, "", ErrImage
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if decoded.ColorModel() == color.CMYKModel {
		return nil, "", ErrImage
	}
	metadata.profile, err = sanitizeColorProfile(metadata.profile, decoded)
	if err != nil {
		return nil, "", err
	}
	normalized := normalizeOrientation(decoded, metadata.orientation)
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	resized := resizeBackground(normalized)
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	var output bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&output, resized, &jpeg.Options{Quality: backgroundJPEGQuality})
	} else {
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		err = encoder.Encode(&output, resized)
	}
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	encoded := output.Bytes()
	colorMetadata, err := metadata.encodeColorMetadata(format)
	if err != nil {
		return nil, "", err
	}
	// Encoding starts from pixels, so all source APP/ancillary chunks are gone.
	// Only the inspected color allowlist is inserted before image data.
	metadataOffset := 33 // PNG signature and complete IHDR.
	if format == "jpeg" {
		metadataOffset = 2
	} // JPEG SOI.
	if len(colorMetadata) > 0 {
		encoded = append(append(append(make([]byte, 0, len(encoded)+len(colorMetadata)), encoded[:metadataOffset]...), colorMetadata...), encoded[metadataOffset:]...)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if len(encoded) > MaxImageBytes {
		return nil, "", ErrImage
	}
	stored, storedFormat, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil || storedFormat != format || stored.Width != resized.Bounds().Dx() || stored.Height != resized.Bounds().Dy() ||
		stored.Width > MaxBackgroundDimension || stored.Height > MaxBackgroundDimension || int64(stored.Width)*int64(stored.Height) > MaxBackgroundPixels {
		return nil, "", ErrImage
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return encoded, format, nil
}

// Preserve aspect ratio without upscaling. Floor both axes so rounding cannot
// exceed either storage limit; keep extremely thin images at least one pixel.
func backgroundDimensions(width, height int) (int, int) {
	scale := math.Min(1, math.Min(float64(MaxBackgroundDimension)/float64(width), float64(MaxBackgroundDimension)/float64(height)))
	scale = math.Min(scale, math.Sqrt(float64(MaxBackgroundPixels)/(float64(width)*float64(height))))
	return max(1, int(math.Floor(float64(width)*scale))), max(1, int(math.Floor(float64(height)*scale)))
}
