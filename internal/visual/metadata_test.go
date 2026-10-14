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
	// Eight-byte TIFF header + two-entry IFD (2 + 2×12 + 4 bytes) = 38.
	tiff := make([]byte, 38)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 2)
	// Orientation (0112), SHORT type 3, count 1: its uint16 value fits inline.
	binary.LittleEndian.PutUint16(tiff[10:12], 0x112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], uint16(orientation))
	// Make (010F), ASCII type 2: its private string follows IFD0 at byte 38.
	binary.LittleEndian.PutUint16(tiff[22:24], 0x10f)
	binary.LittleEndian.PutUint16(tiff[24:26], 2)
	binary.LittleEndian.PutUint32(tiff[26:30], uint32(len(private)+1))
	binary.LittleEndian.PutUint32(tiff[30:34], 38)
	return append(append(tiff, []byte(private)...), 0)
}

// JPEG segment length includes the uint16 length field but excludes FF+marker.
func jpegSegment(marker byte, payload []byte) []byte {
	return append([]byte{0xff, marker, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}, payload...)
}
func insertMetadata(source []byte, format string, metadata []byte) []byte {
	// Insert after PNG signature/IHDR (8+12+13) or JPEG SOI (two bytes).
	offset := 33
	if format == "jpeg" {
		offset = 2
	}
	return append(append(append([]byte{}, source[:offset]...), metadata...), source[offset:]...)
}
func sourceImage(t testing.TB, img image.Image, format string) []byte {
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
				// APP1 (E1) carries EXIF/XMP, APP13 (ED) IPTC, COM (FE) comments.
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
				// Header + one-entry IFD: 8 + 2 + 12 + 4 = 26 bytes,
				// with Orientation (0112), SHORT type 3 and one inline value.
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
	for orientation := 1; orientation <= 8; orientation++ {
		f.Add(tiffOrientation(orientation, "PRIVATE"))
	}
	for _, gray := range []bool{false, true} {
		profile := syntheticColorProfile(gray, "PRIVATE")
		f.Add(profile)
		var source image.Image = image.NewNRGBA(image.Rect(0, 0, 3, 2))
		if gray {
			source = image.NewGray(image.Rect(0, 0, 3, 2))
		}
		for _, format := range []string{"jpeg", "png"} {
			encoded := sourceImage(f, source, format)
			f.Add(encoded)
			metadata, err := (displayMetadata{profile: profile}).encodeColorMetadata(format)
			if err != nil {
				f.Fatal(err)
			}
			if format == "jpeg" {
				metadata = append(metadata, jpegSegment(0xe1, append([]byte("Exif\x00\x00"), tiffOrientation(6, "PRIVATE")...))...)
			} else {
				var orientation bytes.Buffer
				appendPNGChunk(&orientation, "eXIf", tiffOrientation(8, "PRIVATE"))
				metadata = append(metadata, orientation.Bytes()...)
			}
			f.Add(insertMetadata(encoded, format, metadata))
		}
	}
	var srgb bytes.Buffer
	appendPNGChunk(&srgb, "sRGB", []byte{0})
	f.Add(insertMetadata(sourceImage(f, image.NewNRGBA(image.Rect(0, 0, 3, 2)), "png"), "png", srgb.Bytes()))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > MaxImageBytes {
			return
		}
		if orientation, err := readOrientation(body); err == nil && (orientation < 1 || orientation > 8) {
			t.Fatalf("orientation=%d, want 1–8", orientation)
		}
		// Metadata-only fuzzing never allocates pixels based on untrusted dimensions.
		profiles := [][]byte{body}
		for _, format := range []string{"jpeg", "png"} {
			metadata, err := inspectDisplayMetadata(body, format)
			if err != nil {
				continue
			}
			if metadata.orientation < 1 || metadata.orientation > 8 || len(metadata.profile) > maxColorProfileBytes {
				t.Fatal("display metadata exceeds orientation/profile bounds")
			}
			if len(metadata.profile) > 0 {
				if len(metadata.pngColorChunks) != 0 {
					t.Fatal("ICC profile conflicts with retained PNG declarations")
				}
				profiles = append(profiles, metadata.profile)
			}
			for chunks := metadata.pngColorChunks; len(chunks) > 0; {
				if len(chunks) < 12 {
					t.Fatal("truncated retained PNG chunk")
				}
				end := uint64(binary.BigEndian.Uint32(chunks[:4])) + 12
				if end > uint64(len(chunks)) {
					t.Fatal("retained PNG chunk exceeds output bounds")
				}
				switch string(chunks[4:8]) {
				case "sRGB", "gAMA", "cHRM":
				default:
					t.Fatal("retained non-display PNG metadata")
				}
				chunks = chunks[end:]
			}
		}
		for _, profile := range profiles {
			for _, gray := range []bool{false, true} {
				var source image.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))
				if gray {
					source = image.NewGray(image.Rect(0, 0, 1, 1))
				}
				if sanitized, err := sanitizeColorProfile(profile, source); err == nil && len(sanitized) > 0 {
					assertDisplayProfile(t, sanitized, gray)
				}
			}
		}
	})
}

// Check the ICC header/table independently of sanitizer helpers: total size at
// byte 0, device space at 16, tag count at 128, and twelve-byte records at 132.
func assertDisplayProfile(t *testing.T, profile []byte, gray bool) {
	t.Helper()
	if len(profile) < 132 || uint64(binary.BigEndian.Uint32(profile[:4])) != uint64(len(profile)) {
		t.Fatal("sanitized profile has an invalid output length")
	}
	space := "RGB "
	if gray {
		space = "GRAY"
	}
	if string(profile[16:20]) != space {
		t.Fatal("sanitized profile is incompatible with the pixel channels")
	}
	count := uint64(binary.BigEndian.Uint32(profile[128:132]))
	tableEnd := 132 + count*12
	if tableEnd > uint64(len(profile)) {
		t.Fatal("sanitized profile tag table exceeds output bounds")
	}
	for index := uint64(0); index < count; index++ {
		entry := profile[132+index*12 : 144+index*12]
		offset, size := uint64(binary.BigEndian.Uint32(entry[4:8])), uint64(binary.BigEndian.Uint32(entry[8:12]))
		if offset < tableEnd || offset+size > uint64(len(profile)) {
			t.Fatal("sanitized profile tag exceeds output bounds")
		}
		switch string(entry[:4]) {
		case "desc", "cprt", "wtpt", "bkpt", "chad", "lumi":
		case "kTRC":
			if !gray {
				t.Fatal("RGB output retained a grayscale transform")
			}
		case "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC":
			if gray {
				t.Fatal("grayscale output retained an RGB transform")
			}
		default:
			t.Fatal("sanitized profile retained a non-display tag")
		}
	}
}
