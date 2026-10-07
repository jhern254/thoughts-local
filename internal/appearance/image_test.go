package appearance

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestValidateImage(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run("accepts decoded "+format, func(t *testing.T) {
			var b bytes.Buffer
			img := image.NewRGBA(image.Rect(0, 0, 12, 8))
			if format == "png" {
				_ = png.Encode(&b, img)
			} else {
				_ = jpeg.Encode(&b, img, nil)
			}
			got, err := validateImage(b.Bytes())
			if err != nil || got != format {
				t.Fatalf("format = %q, %v; want %s", got, err, format)
			}
		})
	}
	t.Run("rejects unsupported private content", func(t *testing.T) {
		if _, err := validateImage([]byte("<svg>private marker</svg>")); err == nil {
			t.Fatal("accepted unsupported content")
		}
	})
	t.Run("rejects unreasonable dimensions before decoding pixels", func(t *testing.T) {
		var b bytes.Buffer
		_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 8193, 1)))
		if _, err := validateImage(b.Bytes()); err == nil {
			t.Fatal("accepted excessive width")
		}
	})
}

func TestValidateImage_Limits(t *testing.T) {
	t.Run("rejects animated PNG accepted by the standard decoder", func(t *testing.T) {
		original := samplePNG(t)
		chunk := make([]byte, 20)
		binary.BigEndian.PutUint32(chunk[:4], 8)
		copy(chunk[4:8], "acTL")
		binary.BigEndian.PutUint32(chunk[8:12], 1)
		binary.BigEndian.PutUint32(chunk[16:], crc32.ChecksumIEEE(chunk[4:16]))
		animated := append(append(append([]byte{}, original[:33]...), chunk...), original[33:]...)
		if _, err := png.Decode(bytes.NewReader(animated)); err != nil {
			t.Fatalf("fixture is not decodable PNG: %v", err)
		}
		if _, err := validateImage(animated); err == nil {
			t.Fatal("accepted animated PNG")
		}
	})
	t.Run("rejects excessive pixels before allocating a decoded image", func(t *testing.T) {
		body := samplePNG(t)
		binary.BigEndian.PutUint32(body[16:20], 6000)
		binary.BigEndian.PutUint32(body[20:24], 5000)
		binary.BigEndian.PutUint32(body[29:33], crc32.ChecksumIEEE(body[12:29]))
		if _, _, err := image.DecodeConfig(bytes.NewReader(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := validateImage(body); err == nil {
			t.Fatal("accepted 30 million pixels")
		}
	})
	t.Run("accepts maximum width with bounded height", func(t *testing.T) {
		var body bytes.Buffer
		if err := png.Encode(&body, image.NewGray(image.Rect(0, 0, 8192, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := validateImage(body.Bytes()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejects valid image padded beyond byte limit", func(t *testing.T) {
		body := make([]byte, 16*1024*1024+1)
		copy(body, samplePNG(t))
		if _, err := validateImage(body); err == nil {
			t.Fatal("accepted oversized image")
		}
	})
}
