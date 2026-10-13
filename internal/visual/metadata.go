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

func (metadata *displayMetadata) inspectJPEG(body []byte) error {
	var parts [256][]byte
	total := 0
	profileSize := 0
	orientationFound := false
	for offset := 2; offset+2 <= len(body); {
		if body[offset] != 0xff {
			return ErrImage
		}
		marker := body[offset+1]
		if marker == 0xd9 {
			break
		}
		if marker == 0xff {
			offset++
			continue
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			offset += 2
			continue
		}
		if offset+4 > len(body) {
			return ErrImage
		}
		size := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		end := offset + 2 + size
		if size < 2 || end > len(body) {
			return ErrImage
		}
		payload := body[offset+4 : end]
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
		if marker == 0xe2 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")) {
			if len(payload) < 14 {
				return ErrImage
			}
			sequence, count := int(payload[12]), int(payload[13])
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
		if marker == 0xda {
			// Entropy-coded scans escape FF bytes as FF00 and use restart
			// markers. Inspect APP segments between scans as well as before
			// the first scan, so a late color declaration cannot be lost.
			for offset+1 < len(body) {
				if body[offset] != 0xff {
					offset++
					continue
				}
				next := body[offset+1]
				if next == 0 || next >= 0xd0 && next <= 0xd7 {
					offset += 2
					continue
				}
				if next == 0xff {
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
		for sequence := 1; sequence <= total; sequence++ {
			if parts[sequence] == nil {
				return ErrImage
			}
			metadata.profile = append(metadata.profile, parts[sequence]...)
		}
	}
	return nil
}

func (metadata *displayMetadata) inspectPNG(body []byte) error {
	orientationFound, profileFound, srgbFound := false, false, false
	grayAlpha := false
	var gamma, chromaticity, srgb []byte
	for offset := 8; offset+12 <= len(body); {
		size := uint64(binary.BigEndian.Uint32(body[offset : offset+4]))
		end := uint64(offset) + 12 + size
		if end > uint64(len(body)) {
			return ErrImage
		}
		chunk := body[offset:int(end)]
		payload := chunk[8 : len(chunk)-4]
		// The decoder checks all CRCs too; validate inspected metadata before use.
		if crc32.ChecksumIEEE(chunk[4:len(chunk)-4]) != binary.BigEndian.Uint32(chunk[len(chunk)-4:]) {
			return ErrImage
		}
		switch string(chunk[4:8]) {
		case "IHDR":
			if len(payload) != 13 {
				return ErrImage
			}
			// Go expands gray+alpha into NRGBA, which loses the original
			// channel model. Its profile cannot be copied to RGB output.
			grayAlpha = payload[9] == 4
		case "acTL", "fcTL", "fdAT", "cICP", "mDCV", "cLLI":
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
			separator := bytes.IndexByte(payload, 0)
			if separator < 1 || separator > 79 || separator+2 > len(payload) || payload[separator+1] != 0 {
				return ErrImage
			}
			reader, err := zlib.NewReader(bytes.NewReader(payload[separator+2:]))
			if err != nil {
				return ErrImage
			}
			profile, readErr := io.ReadAll(io.LimitReader(reader, maxColorProfileBytes+1))
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil || len(profile) == 0 || len(profile) > maxColorProfileBytes {
				return ErrImage
			}
			metadata.profile = profile
		case "sRGB":
			if srgbFound || len(payload) != 1 || payload[0] > 3 {
				return ErrImage
			}
			srgbFound = true
			srgb = chunk
		case "gAMA":
			if gamma != nil || len(payload) != 4 || binary.BigEndian.Uint32(payload) == 0 {
				return ErrImage
			}
			gamma = chunk
		case "cHRM":
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
		if string(chunk[4:8]) == "IEND" {
			break
		}
	}
	if profileFound && (srgbFound || grayAlpha) {
		return ErrImage
	} // Conflicting color declarations or an unsupported source color model.
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
			payload := append([]byte{'I', 'C', 'C', 0, 0}, compressed.Bytes()...)
			appendPNGChunk(&output, "iCCP", payload)
		}
	}
	return output.Bytes(), nil
}
