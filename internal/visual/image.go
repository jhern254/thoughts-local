package visual

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"math"

	"golang.org/x/image/draw"
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
	var metadata []byte
	// The standard PNG decoder ignores animation chunks. Reject APNG before
	// preserving the original bytes, otherwise browsers would animate them.
	if format == "png" {
		for offset := 8; offset+12 <= len(body); {
			size := int64(binary.BigEndian.Uint32(body[offset : offset+4]))
			end := int64(offset) + 12 + size
			if end > int64(len(body)) {
				return nil, "", ErrImage
			}
			if string(body[offset+4:offset+8]) == "acTL" {
				return nil, "", ErrImage
			}
			if string(body[offset+4:offset+8]) == "eXIf" {
				metadata = body[offset:int(end)]
			}
			offset = int(end)
		}
	}
	decoded, actual, err := image.Decode(bytes.NewReader(body))
	if err != nil || actual != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, "", ErrImage
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	width, height := backgroundDimensions(config.Width, config.Height)
	if width == config.Width && height == config.Height {
		return body, format, nil
	}
	// RGBA selects the scaler's specialized path, avoiding per-pixel interface
	// allocations. PNG encoding retains transparency from premultiplied pixels.
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), decoded, decoded.Bounds(), draw.Src, nil)
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
	// Preserve EXIF when resizing: browsers apply its orientation, whereas
	// Go decodes the stored pixel axes. Dropping it would rotate phone pictures
	// relative to their preview. No metadata is interpreted or logged here.
	metadataOffset := 33 // PNG signature and complete IHDR chunk.
	if format == "jpeg" {
		metadata = jpegEXIF(body)
		metadataOffset = 2
	} // JPEG SOI marker.
	if len(metadata) > 0 {
		encoded = append(append(append(make([]byte, 0, len(encoded)+len(metadata)), encoded[:metadataOffset]...), metadata...), encoded[metadataOffset:]...)
	}
	if len(encoded) > MaxImageBytes {
		return nil, "", ErrImage
	}
	return encoded, format, nil
}

func jpegEXIF(body []byte) []byte {
	for offset := 2; offset+4 <= len(body); {
		if body[offset] != 0xff {
			return nil
		}
		marker := body[offset+1]
		if marker == 0xda || marker == 0xd9 {
			return nil
		} // Scan data or end of image.
		if marker == 0xff {
			offset++
			continue
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			offset += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		end := offset + 2 + size
		if size < 2 || end > len(body) {
			return nil
		}
		if marker == 0xe1 && bytes.HasPrefix(body[offset+4:end], []byte("Exif\x00\x00")) {
			return body[offset:end]
		}
		offset = end
	}
	return nil
}

// Preserve aspect ratio without upscaling. Floor both axes so rounding cannot
// exceed either storage limit; keep extremely thin images at least one pixel.
func backgroundDimensions(width, height int) (int, int) {
	scale := math.Min(1, math.Min(float64(MaxBackgroundDimension)/float64(width), float64(MaxBackgroundDimension)/float64(height)))
	scale = math.Min(scale, math.Sqrt(float64(MaxBackgroundPixels)/(float64(width)*float64(height))))
	return max(1, int(math.Floor(float64(width)*scale))), max(1, int(math.Floor(float64(height)*scale)))
}
