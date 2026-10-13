package visual

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func tiffOrientation(orientation int, private string) []byte {
	// A private ASCII tag points beyond IFD0. The reader never needs its data.
	tiff := make([]byte, 38)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 2)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], uint16(orientation))
	binary.LittleEndian.PutUint16(tiff[22:24], 0x10f)
	binary.LittleEndian.PutUint16(tiff[24:26], 2)
	binary.LittleEndian.PutUint32(tiff[26:30], uint32(len(private)+1))
	binary.LittleEndian.PutUint32(tiff[30:34], 38)
	return append(append(tiff, []byte(private)...), 0)
}
func jpegSegment(marker byte, payload []byte) []byte {
	return append([]byte{0xff, marker, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}, payload...)
}
func insertMetadata(source []byte, format string, metadata []byte) []byte {
	offset := 33
	if format == "jpeg" {
		offset = 2
	}
	return append(append(append([]byte{}, source[:offset]...), metadata...), source[offset:]...)
}
func sourceImage(t *testing.T, img image.Image, format string) []byte {
	t.Helper()
	var output bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 100})
	} else {
		err = png.Encode(&output, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestPrepareImage_Metadata(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run(format+" discards private metadata even without resizing", func(t *testing.T) {
			marker := "PRIVATE GPS camera serial timestamp owner thumbnail"
			img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
			img.SetNRGBA(0, 0, color.NRGBA{40, 80, 160, 128})
			tiff := tiffOrientation(6, marker)
			var metadata bytes.Buffer
			if format == "jpeg" {
				metadata.Write(jpegSegment(0xe1, append([]byte("Exif\x00\x00"), tiff...)))
				metadata.Write(jpegSegment(0xe1, []byte("http://ns.adobe.com/xap/1.0/\x00"+marker)))
				metadata.Write(jpegSegment(0xed, []byte("IPTC "+marker)))
				metadata.Write(jpegSegment(0xfe, []byte(marker)))
			} else {
				appendPNGChunk(&metadata, "eXIf", tiff)
				appendPNGChunk(&metadata, "tEXt", []byte("Comment\x00"+marker))
				var compressed bytes.Buffer
				writer := zlib.NewWriter(&compressed)
				writer.Write([]byte(marker))
				writer.Close()
				appendPNGChunk(&metadata, "zTXt", append([]byte("Comment\x00\x00"), compressed.Bytes()...))
				appendPNGChunk(&metadata, "iTXt", []byte("Comment\x00\x00\x00\x00\x00"+marker))
			}
			body := insertMetadata(sourceImage(t, img, format), format, metadata.Bytes())
			prepared, _, err := prepareImage(t.Context(), body)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{marker, "Exif\x00\x00", "eXIf", "tEXt", "zTXt", "iTXt", "IPTC", "http://ns.adobe.com"} {
				if bytes.Contains(prepared, []byte(private)) {
					t.Fatalf("retained %q", private)
				}
			}
			decoded, _, err := image.Decode(bytes.NewReader(prepared))
			if err != nil {
				t.Fatal(err)
			}
			if got := decoded.Bounds().Size(); got != (image.Point{2, 3}) {
				t.Fatalf("size=%v, want (2,3)", got)
			}
			if format == "png" && decoded.At(1, 0) != (color.NRGBA{40, 80, 160, 128}) {
				t.Fatalf("rotated pixel=%v, want unchanged RGBA", decoded.At(1, 0))
			}
		})
	}
	t.Run("malformed metadata returns fixed safe error", func(t *testing.T) {
		for _, tiff := range [][]byte{{}, {'I', 'I', 42, 0, 255, 255, 255, 255}, tiffOrientation(9, "PRIVATE")} {
			body := insertMetadata(samplePNG(t), "png", func() []byte { var chunk bytes.Buffer; appendPNGChunk(&chunk, "eXIf", tiff); return chunk.Bytes() }())
			if _, _, err := prepareImage(t.Context(), body); !errors.Is(err, ErrImage) {
				t.Fatalf("error=%v, want safe image error", err)
			}
		}
	})
}

func TestNormalizeOrientation(t *testing.T) {
	// Literal displayed rows, independent of the production coordinate mapping.
	rows := []string{"", "ABC/DEF", "CBA/FED", "FED/CBA", "DEF/ABC", "AD/BE/CF", "DA/EB/FC", "FC/EB/DA", "CF/BE/AD"}
	for orientation := 1; orientation <= 8; orientation++ {
		t.Run(fmt.Sprint(orientation), func(t *testing.T) {
			source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
			for i := 0; i < 6; i++ {
				source.SetNRGBA(i%3, i/3, color.NRGBA{R: byte('A' + i), A: 255})
			}
			displayed := normalizeOrientation(source, orientation)
			var got bytes.Buffer
			for y := 0; y < displayed.Bounds().Dy(); y++ {
				if y > 0 {
					got.WriteByte('/')
				}
				for x := 0; x < displayed.Bounds().Dx(); x++ {
					got.WriteByte(color.NRGBAModel.Convert(displayed.At(x, y)).(color.NRGBA).R)
				}
			}
			if got.String() != rows[orientation] {
				t.Fatalf("rows=%s, want %s", got.String(), rows[orientation])
			}
			for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
				tiff := make([]byte, 26)
				copy(tiff, "II")
				if order == binary.BigEndian {
					copy(tiff, "MM")
				}
				order.PutUint16(tiff[2:4], 42)
				order.PutUint32(tiff[4:8], 8)
				order.PutUint16(tiff[8:10], 1)
				order.PutUint16(tiff[10:12], 0x112)
				order.PutUint16(tiff[12:14], 3)
				order.PutUint32(tiff[14:18], 1)
				order.PutUint16(tiff[18:20], uint16(orientation))
				if got, err := readOrientation(tiff); err != nil || got != orientation {
					t.Fatalf("orientation=%d,%v, want %d", got, err, orientation)
				}
			}
		})
	}
}

func FuzzDisplayMetadata(f *testing.F) {
	f.Add([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte("private malformed input"))
	f.Add(syntheticColorProfile(false, "PRIVATE"))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > MaxImageBytes {
			return
		}
		_, _ = readOrientation(body)
		_, _ = inspectDisplayMetadata(body, "jpeg")
		_, _ = inspectDisplayMetadata(body, "png")
		_, _ = sanitizeColorProfile(body, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	})
}
