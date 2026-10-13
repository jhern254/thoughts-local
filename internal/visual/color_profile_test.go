package visual

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"testing"
)

// These profiles are constructed here from numeric matrix/TRC data, not copied
// from licensed profile files. The RGB fixture describes Adobe RGB primaries
// with a 2.2 transfer curve; browsers, rather than this test, perform conversion.
func syntheticColorProfile(gray bool, private string) []byte {
	return syntheticColorProfileVersion(gray, private, 4)
}
func syntheticColorProfileVersion(gray bool, private string, major byte) []byte {
	profile := make([]byte, 132)
	profile[8] = major
	profile[9] = 0x30
	copy(profile[12:24], "mntrRGB XYZ ")
	if gray {
		copy(profile[16:20], "GRAY")
	}
	copy(profile[36:40], "acsp")
	binary.BigEndian.PutUint32(profile[64:68], 1)
	for index, value := range []float64{.9642, 1, .8249} {
		binary.BigEndian.PutUint32(profile[68+index*4:72+index*4], uint32(math.Round(value*65536)))
	}
	copy(profile[48:52], "PRIV")
	copy(profile[80:84], "USER")
	xyz := func(values ...float64) []byte {
		data := make([]byte, 20)
		copy(data, "XYZ ")
		for index, value := range values {
			binary.BigEndian.PutUint32(data[8+index*4:12+index*4], uint32(int32(math.Round(value*65536))))
		}
		return data
	}
	curve := make([]byte, 16)
	copy(curve, "para")
	binary.BigEndian.PutUint32(curve[12:16], uint32(math.Round(2.2*65536)))
	if major == 2 {
		curve = make([]byte, 14)
		copy(curve, "curv")
		binary.BigEndian.PutUint32(curve[8:12], 1)
		binary.BigEndian.PutUint16(curve[12:14], 563)
	}

	names := []string{"desc", "cprt", "wtpt", "priv"}
	tags := map[string][]byte{
		"desc": fixedProfileText(major, "desc", private),
		"cprt": fixedProfileText(major, "cprt", private),
		"wtpt": xyz(.9642, 1, .8249),
		"priv": append([]byte("text\x00\x00\x00\x00"), []byte(private)...),
	}
	if gray {
		names = append(names, "kTRC")
		tags["kTRC"] = curve
	} else {
		names = append(names, "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC")
		tags["rXYZ"] = xyz(.6097559, .3111242, .0194811)
		tags["gXYZ"] = xyz(.2052401, .625656, .0608902)
		tags["bXYZ"] = xyz(.149224, .0632197, .7448387)
		tags["rTRC"] = curve
		tags["gTRC"] = curve
		tags["bTRC"] = curve
	}
	binary.BigEndian.PutUint32(profile[128:132], uint32(len(names)))
	profile = append(profile, make([]byte, 12*len(names))...)
	for index, name := range names {
		entry := profile[132+index*12 : 144+index*12]
		copy(entry, name)
		binary.BigEndian.PutUint32(entry[4:8], uint32(len(profile)))
		binary.BigEndian.PutUint32(entry[8:12], uint32(len(tags[name])))
		profile = append(profile, tags[name]...)
		for len(profile)%4 != 0 {
			profile = append(profile, 0)
		}
	}
	binary.BigEndian.PutUint32(profile[:4], uint32(len(profile)))
	return profile
}

func profileTag(profile []byte, name string) []byte {
	count := int(binary.BigEndian.Uint32(profile[128:132]))
	for index := 0; index < count; index++ {
		entry := profile[132+index*12 : 144+index*12]
		if string(entry[:4]) == name {
			offset, size := int(binary.BigEndian.Uint32(entry[4:8])), int(binary.BigEndian.Uint32(entry[8:12]))
			return profile[offset : offset+size]
		}
	}
	return nil
}

func TestPrepareImage_Color(t *testing.T) {
	for _, gray := range []bool{false, true} {
		for _, format := range []string{"jpeg", "png"} {
			t.Run(format+map[bool]string{true: " gray", false: " RGB"}[gray], func(t *testing.T) {
				var source image.Image = image.NewNRGBA(image.Rect(0, 0, 8, 6))
				if gray {
					img := image.NewGray(image.Rect(0, 0, 8, 6))
					for index := range img.Pix {
						img.Pix[index] = 128
					}
					source = img
				}
				profile := syntheticColorProfile(gray, "PRIVATE GPS serial timestamp")
				metadata, err := (displayMetadata{profile: profile}).encodeColorMetadata(format)
				if err != nil {
					t.Fatal(err)
				}
				prepared, _, err := prepareImage(t.Context(), insertMetadata(sourceImage(t, source, format), format, metadata))
				if err != nil {
					t.Fatal(err)
				}
				stored, err := inspectDisplayMetadata(prepared, format)
				if err != nil {
					t.Fatal(err)
				}
				decoded, _, err := image.Decode(bytes.NewReader(prepared))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = sanitizeColorProfile(stored.profile, decoded); err != nil {
					t.Fatalf("stored profile is incompatible or invalid: %v", err)
				}
				if bytes.Contains(stored.profile, []byte("PRIVATE")) || bytes.Contains(stored.profile, []byte("PRIV")) || bytes.Contains(stored.profile, []byte("USER")) {
					t.Fatal("profile retained identifying metadata")
				}
				names := []string{"wtpt", "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC"}
				if gray {
					names = []string{"wtpt", "kTRC"}
				}
				for _, name := range names {
					if !bytes.Equal(profileTag(stored.profile, name), profileTag(profile, name)) {
						t.Fatalf("changed %s color transform", name)
					}
				}
			})
		}
	}
	t.Run("v2 curve profiles retain their numeric interpretation", func(t *testing.T) {
		profile := syntheticColorProfileVersion(false, "PRIVATE", 2)
		stored, err := sanitizeColorProfile(profile, image.NewRGBA(image.Rect(0, 0, 1, 1)))
		if err != nil || stored[8] != 2 || string(profileTag(stored, "desc")[:4]) != "desc" || string(profileTag(stored, "cprt")[:4]) != "text" {
			t.Fatalf("v2 profile encoding error=%v", err)
		}
		if !bytes.Equal(profileTag(profile, "rTRC"), profileTag(stored, "rTRC")) {
			t.Fatal("changed v2 curve")
		}
		curve := make([]byte, 14)
		copy(curve, "curv")
		binary.BigEndian.PutUint32(curve[8:12], 1)
		binary.BigEndian.PutUint16(curve[12:14], 563)
		if !validNumericColorTag("rTRC", curve) {
			t.Fatal("valid v2 gamma curve rejected")
		}
	})

	t.Run("multipart JPEG profile is assembled in sequence order", func(t *testing.T) {
		profile := syntheticColorProfile(false, string(bytes.Repeat([]byte("PRIVATE"), 12000)))
		encoded, err := (displayMetadata{profile: profile}).encodeColorMetadata("jpeg")
		if err != nil {
			t.Fatal(err)
		}
		firstEnd := 2 + int(binary.BigEndian.Uint16(encoded[2:4]))
		reordered := append(append([]byte{}, encoded[firstEnd:]...), encoded[:firstEnd]...)
		body := insertMetadata(sourceImage(t, image.NewRGBA(image.Rect(0, 0, 8, 6)), "jpeg"), "jpeg", reordered)
		prepared, _, err := prepareImage(t.Context(), body)
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := inspectDisplayMetadata(prepared, "jpeg")
		if err != nil || !bytes.Equal(profileTag(metadata.profile, "rXYZ"), profileTag(profile, "rXYZ")) {
			t.Fatalf("profile assembly changed transform: %v", err)
		}
		if bytes.Contains(metadata.profile, []byte("PRIVATE")) {
			t.Fatal("multipart profile retained private text")
		}
	})
	t.Run("incompatible or unsupported profiles are rejected", func(t *testing.T) {
		for _, space := range []string{"CMYK", "GRAY", "Lab "} {
			profile := syntheticColorProfile(false, "PRIVATE")
			copy(profile[16:20], space)
			if _, err := sanitizeColorProfile(profile, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != ErrImage {
				t.Fatalf("%s error=%v, want safe image rejection", space, err)
			}
		}
		profile := syntheticColorProfile(false, "PRIVATE")
		copy(profile[132:136], "A2B0")
		if _, err := sanitizeColorProfile(profile, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != ErrImage {
			t.Fatalf("LUT profile error=%v", err)
		}
	})
	t.Run("PNG effective color declarations remain without private chunks", func(t *testing.T) {
		for _, kind := range []string{"sRGB", "gAMA"} {
			var chunks bytes.Buffer
			payload := []byte{1}
			if kind == "gAMA" {
				payload = make([]byte, 4)
				binary.BigEndian.PutUint32(payload, 45455)
			}
			appendPNGChunk(&chunks, kind, payload)
			appendPNGChunk(&chunks, "tEXt", []byte("Comment\x00PRIVATE"))
			prepared, _, err := prepareImage(t.Context(), insertMetadata(samplePNG(t), "png", chunks.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := inspectDisplayMetadata(prepared, "png")
			if err != nil || !bytes.Contains(metadata.pngColorChunks, []byte(kind)) || bytes.Contains(prepared, []byte("PRIVATE")) {
				t.Fatalf("%s color metadata=%v, error=%v", kind, metadata.pngColorChunks, err)
			}
		}
	})
	t.Run("grayscale alpha cannot acquire an RGB profile during reencoding", func(t *testing.T) {
		// PNG color type 4 has gray+alpha samples, even though Go decodes
		// it into NRGBA. An RGB profile is incompatible with the source.
		var source bytes.Buffer
		source.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
		appendPNGChunk(&source, "IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 4, 0, 0, 0})
		metadata, err := (displayMetadata{profile: syntheticColorProfile(false, "PRIVATE")}).encodeColorMetadata("png")
		if err != nil {
			t.Fatal(err)
		}
		source.Write(metadata)
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		writer.Write([]byte{0, 128, 128})
		writer.Close()
		appendPNGChunk(&source, "IDAT", compressed.Bytes())
		appendPNGChunk(&source, "IEND", nil)
		if _, _, err := prepareImage(t.Context(), source.Bytes()); err != ErrImage {
			t.Fatalf("error=%v, want safe incompatible-profile rejection", err)
		}
	})

	t.Run("oversized compressed PNG profile is rejected", func(t *testing.T) {
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		writer.Write(make([]byte, maxColorProfileBytes+1))
		writer.Close()
		var chunk bytes.Buffer
		appendPNGChunk(&chunk, "iCCP", append([]byte("Profile\x00\x00"), compressed.Bytes()...))
		if _, _, err := prepareImage(t.Context(), insertMetadata(samplePNG(t), "png", chunk.Bytes())); err != ErrImage {
			t.Fatalf("error=%v, want safe image rejection", err)
		}
	})
	t.Run("missing and duplicate JPEG ICC parts are rejected", func(t *testing.T) {
		for _, suffix := range [][]byte{nil, jpegSegment(0xe2, append([]byte("ICC_PROFILE\x00\x01\x02"), []byte("PRIVATE")...))} {
			metadata := jpegSegment(0xe2, append([]byte("ICC_PROFILE\x00\x01\x02"), []byte("PRIVATE")...))
			metadata = append(metadata, suffix...)
			if _, _, err := prepareImage(t.Context(), insertMetadata(sourceImage(t, image.NewRGBA(image.Rect(0, 0, 1, 1)), "jpeg"), "jpeg", metadata)); err != ErrImage {
				t.Fatalf("error=%v, want safe image rejection", err)
			}
		}
	})

	t.Run("16 bit PNG and alpha keep precision during orientation", func(t *testing.T) {
		source := image.NewNRGBA64(image.Rect(0, 0, 2, 3))
		want := color.NRGBA64{12345, 23456, 34567, 45678}
		source.SetNRGBA64(0, 0, want)
		var chunks bytes.Buffer
		appendPNGChunk(&chunks, "eXIf", tiffOrientation(8, "PRIVATE"))
		prepared, _, err := prepareImage(t.Context(), insertMetadata(sourceImage(t, source, "png"), "png", chunks.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		decoded, _, err := image.Decode(bytes.NewReader(prepared))
		if err != nil {
			t.Fatal(err)
		}
		if got := decoded.At(0, 1); got != want {
			t.Fatalf("pixel=%v, want %v", got, want)
		}
	})
}
