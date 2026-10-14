package visual

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"io"
)

// A compressed profile can expand independently of the decoded image budget.
const maxColorProfileBytes = 4 << 20
const jpegICCChunkBytes = 65519 // APP2's uint16 length minus its header and ICC sequence fields.

// Source metadata is not copied wholesale. Only orientation and the effective
// color description survive inspection; orientation is subsequently discarded.
type displayMetadata struct {
	orientation    int
	profile        []byte
	pngColorChunks []byte
}

// prepareImage has already validated the JPEG/PNG signature and source bounds.
// This pass inspects display declarations; the standard decoder validates pixels.
func inspectDisplayMetadata(body []byte, format string) (displayMetadata, error) {
	metadata := displayMetadata{orientation: 1}
	var err error
	if format == "jpeg" {
		err = metadata.inspectJPEG(body)
	} else {
		err = metadata.inspectPNG(body)
	}
	return metadata, err
}

// Marker meanings follow the JPEG standard, as implemented by Go:
// https://go.dev/src/image/jpeg/reader.go
func (metadata *displayMetadata) inspectJPEG(body []byte) error {
	// ICC sequence/count fields are bytes: slots 1–255 hold parts, slot 0 is unused.
	var parts [256][]byte
	total := 0
	profileSize := 0
	orientationFound := false
	// Skip the two-byte Start Of Image marker (FF D8).
	for offset := 2; offset+2 <= len(body); {
		// Outside compressed scan data, every JPEG marker starts with FF.
		if body[offset] != 0xff {
			return ErrImage
		}
		marker := body[offset+1]
		if marker == 0xd9 { // FF D9: End Of Image; no more display metadata.
			break
		}
		if marker == 0xff { // Repeated FF fill bytes precede the actual marker code.
			offset++
			continue
		}
		// TEM (01) and restart markers RST0–RST7 (D0–D7) have no length/payload.
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			offset += 2
			continue
		}
		// Other segments need two marker bytes followed by a big-endian uint16 length.
		if offset+4 > len(body) {
			return ErrImage
		}
		// Length includes its own two bytes, but excludes the two-byte marker.
		size := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		end := offset + 2 + size
		if size < 2 || end > len(body) {
			return ErrImage
		}
		payload := body[offset+4 : end]
		// APP1 (E1) carries EXIF after its six-byte Exif+NUL+NUL identifier.
		if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			if orientationFound {
				return ErrImage
			}
			orientationFound = true
			orientation, err := readOrientation(payload[6:])
			if err != nil {
				return err
			}
			metadata.orientation = orientation
		}
		// APP2 (E2) ICC data starts with a 12-byte identifier, then one-byte
		// sequence/count fields. The profile may span out-of-order segments.
		if marker == 0xe2 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")) {
			if len(payload) < 14 {
				return ErrImage
			}
			sequence, count := int(payload[12]), int(payload[13])
			// Reject ambiguous duplicates/counts rather than assemble a different profile.
			if count == 0 || sequence == 0 || sequence > count || parts[sequence] != nil || total != 0 && total != count {
				return ErrImage
			}
			total = count
			parts[sequence] = payload[14:]
			profileSize += len(payload) - 14
			if profileSize > maxColorProfileBytes {
				return ErrImage
			}
		}
		offset = end
		if marker == 0xda { // FF DA: Start Of Scan; compressed pixels follow its header.
			// Entropy-coded scans escape FF bytes as FF00 and use restart
			// markers. Inspect APP segments between scans as well as before
			// the first scan, so a late color declaration cannot be lost.
			for offset+1 < len(body) {
				if body[offset] != 0xff {
					offset++
					continue
				}
				next := body[offset+1]
				// FF00 represents a literal FF pixel-data byte; restart markers
				// stay within the scan rather than starting a metadata segment.
				if next == 0 || next >= 0xd0 && next <= 0xd7 {
					offset += 2
					continue
				}
				if next == 0xff { // Skip fill bytes until the next marker code.
					offset++
					continue
				}
				break
			}
		}
	}
	if total > 0 {
		if profileSize == 0 {
			return ErrImage
		}
		metadata.profile = make([]byte, 0, profileSize)
		// Every declared part must exist; physical file order is not profile order.
		for sequence := 1; sequence <= total; sequence++ {
			if parts[sequence] == nil {
				return ErrImage
			}
			metadata.profile = append(metadata.profile, parts[sequence]...)
		}
	}
	return nil
}

// Chunk layouts and display declaration precedence follow the PNG specification:
// https://www.w3.org/TR/png-3/
func (metadata *displayMetadata) inspectPNG(body []byte) error {
	orientationFound, profileFound, srgbFound := false, false, false
	grayAlpha := false
	var gamma, chromaticity, srgb []byte
	// Skip the eight-byte PNG signature. Each chunk has uint32 length,
	// four-byte type, payload, and uint32 CRC: twelve bytes plus payload.
	for offset := 8; offset+12 <= len(body); {
		size := uint64(binary.BigEndian.Uint32(body[offset : offset+4]))
		// Widen before addition so attacker-controlled uint32 lengths cannot wrap.
		end := uint64(offset) + 12 + size
		if end > uint64(len(body)) {
			return ErrImage
		}
		chunk := body[offset:int(end)]
		payload := chunk[8 : len(chunk)-4]
		// CRC covers the chunk type and payload, excluding length and CRC itself.
		// The decoder checks CRCs too; validate inspected metadata before use.
		if crc32.ChecksumIEEE(chunk[4:len(chunk)-4]) != binary.BigEndian.Uint32(chunk[len(chunk)-4:]) {
			return ErrImage
		}
		switch string(chunk[4:8]) {
		case "IHDR": // Fixed 13-byte header: dimensions, depth, color type and methods.
			if len(payload) != 13 {
				return ErrImage
			}
			// Go expands gray+alpha into NRGBA, which loses the original
			// channel model. Its profile cannot be copied to RGB output.
			grayAlpha = payload[9] == 4 // Byte 9 is color type; 4 means gray+alpha.
		case "acTL", "fcTL", "fdAT", "cICP", "mDCV", "cLLI":
			// acTL/fcTL/fdAT declare APNG animation. cICP identifies color
			// encoding; mDCV/cLLI carry HDR mastering/light information.
			// These need display handling beyond the supported static subset.
			return ErrImage // Animation/HDR requires another display pipeline.
		case "eXIf":
			if orientationFound {
				return ErrImage
			}
			orientationFound = true
			orientation, err := readOrientation(payload)
			if err != nil {
				return err
			}
			metadata.orientation = orientation
		case "iCCP":
			if profileFound {
				return ErrImage
			}
			profileFound = true
			// iCCP stores a 1–79-byte name, NUL, compression method 0 (zlib),
			// then compressed ICC bytes. The name is discarded, not used as a path.
			separator := bytes.IndexByte(payload, 0)
			if separator < 1 || separator > 79 || separator+2 > len(payload) || payload[separator+1] != 0 {
				return ErrImage
			}
			reader, err := zlib.NewReader(bytes.NewReader(payload[separator+2:]))
			if err != nil {
				return ErrImage
			}
			// One extra byte distinguishes an oversized expansion from a valid limit.
			profile, readErr := io.ReadAll(io.LimitReader(reader, maxColorProfileBytes+1))
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil || len(profile) == 0 || len(profile) > maxColorProfileBytes {
				return ErrImage
			}
			metadata.profile = profile
		case "sRGB": // A single rendering-intent byte: 0 perceptual, 1 relative, 2 saturation, 3 absolute.
			if srgbFound || len(payload) != 1 || payload[0] > 3 {
				return ErrImage
			}
			srgbFound = true
			srgb = chunk
		case "gAMA": // Positive uint32 image gamma, scaled by 100,000.
			if gamma != nil || len(payload) != 4 || binary.BigEndian.Uint32(payload) == 0 {
				return ErrImage
			}
			gamma = chunk
		case "cHRM":
			// Four (x,y) chromaticity pairs (white/red/green/blue), each using
			// two uint32 values scaled by 100,000. Require y>0 and x+y<=1.
			if chromaticity != nil || len(payload) != 32 {
				return ErrImage
			}
			for index := 0; index < 32; index += 8 {
				x, y := uint64(binary.BigEndian.Uint32(payload[index:index+4])), uint64(binary.BigEndian.Uint32(payload[index+4:index+8]))
				if y == 0 || x+y > 100000 {
					return ErrImage
				}
			}
			chromaticity = chunk
		}
		offset = int(end)
		if string(chunk[4:8]) == "IEND" { // End Of Image; trailing bytes are not retained.
			break
		}
	}
	if profileFound && (srgbFound || grayAlpha) {
		return ErrImage
	} // Conflicting color declarations or an unsupported source color model.
	// Retain the effective declaration: ICC takes precedence over gAMA/cHRM;
	// sRGB also makes those fallback declarations unnecessary.
	if profileFound {
		return nil
	}
	if srgbFound {
		metadata.pngColorChunks = srgb
		return nil
	}
	metadata.pngColorChunks = append(append([]byte{}, gamma...), chromaticity...)
	return nil
}

// Write length/type/payload/CRC in PNG wire order; CRC excludes the length field.
func appendPNGChunk(output *bytes.Buffer, kind string, payload []byte) {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(payload)))
	output.Write(size[:])
	output.WriteString(kind)
	output.Write(payload)
	checksum := crc32.NewIEEE()
	checksum.Write([]byte(kind))
	checksum.Write(payload)
	binary.BigEndian.PutUint32(size[:], checksum.Sum32())
	output.Write(size[:])
}

func (metadata displayMetadata) encodeColorMetadata(format string) ([]byte, error) {
	var output bytes.Buffer
	if format == "jpeg" {
		// Split the sanitized profile into APP2-sized pieces, numbering from one.
		count := (len(metadata.profile) + jpegICCChunkBytes - 1) / jpegICCChunkBytes
		for offset, sequence := 0, 1; offset < len(metadata.profile); sequence++ {
			end := min(offset+jpegICCChunkBytes, len(metadata.profile))
			payload := append([]byte("ICC_PROFILE\x00"), byte(sequence), byte(count))
			payload = append(payload, metadata.profile[offset:end]...)
			output.Write([]byte{0xff, 0xe2, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)})
			output.Write(payload)
			offset = end
		}
	} else {
		output.Write(metadata.pngColorChunks)
		if len(metadata.profile) > 0 {
			var compressed bytes.Buffer
			writer := zlib.NewWriter(&compressed)
			if _, err := writer.Write(metadata.profile); err != nil {
				return nil, err
			}
			if err := writer.Close(); err != nil {
				return nil, err
			}
			// Fixed keyword + NUL + zlib method 0; never retain the source keyword.
			payload := append([]byte{'I', 'C', 'C', 0, 0}, compressed.Bytes()...)
			appendPNGChunk(&output, "iCCP", payload)
		}
	}
	return output.Bytes(), nil
}
