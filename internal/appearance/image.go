package appearance

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

const (
	// Bound compressed input and decoded memory independently.
	MaxImageBytes = 16 << 20
	MaxDimension  = 8192
	MaxPixels     = 24_000_000
)

var ErrImage = errors.New("choose a valid, non-animated JPEG or PNG within the image limits")
var ErrDarkness = errors.New("background darkness must be between 0 and 95")
var ErrBusy = errors.New("an appearance change is already in progress")

func validateImage(body []byte) (string, error) {
	if len(body) > MaxImageBytes {
		return "", ErrImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 ||
		config.Width > MaxDimension || config.Height > MaxDimension || int64(config.Width)*int64(config.Height) > MaxPixels {
		return "", ErrImage
	}
	// The standard PNG decoder ignores animation chunks. Reject APNG before
	// preserving the original bytes, otherwise browsers would animate them.
	if format == "png" {
		for offset := 8; offset+12 <= len(body); {
			size := int64(binary.BigEndian.Uint32(body[offset : offset+4]))
			end := int64(offset) + 12 + size
			if end > int64(len(body)) {
				return "", ErrImage
			}
			if string(body[offset+4:offset+8]) == "acTL" {
				return "", ErrImage
			}
			offset = int(end)
		}
	}
	decoded, actual, err := image.Decode(bytes.NewReader(body))
	if err != nil || actual != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return "", ErrImage
	}
	return format, nil
}
