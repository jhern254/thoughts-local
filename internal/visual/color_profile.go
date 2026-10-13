package visual

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"image"
	"image/color"
)

const maxColorProfileTags = 4096

// This is a container sanitizer, not a color-management engine. Matrix/TRC
// profiles describe encoded RGB/gray samples without converting them. Their
// numeric transforms remain intact while descriptive/private tags and device
// identification are removed. LUT/HDR/CMYK profiles require a real CMM and are
// rejected rather than attached to incompatible output or silently discarded.
func sanitizeColorProfile(profile []byte, source image.Image) ([]byte, error) {
	if len(profile) == 0 {
		return nil, nil
	}
	if len(profile) < 132 || len(profile) > maxColorProfileBytes || int64(binary.BigEndian.Uint32(profile[:4])) != int64(len(profile)) || string(profile[36:40]) != "acsp" {
		return nil, ErrImage
	}
	major := profile[8]
	if major != 2 && major != 4 || string(profile[20:24]) != "XYZ " || (string(profile[12:16]) != "mntr" && string(profile[12:16]) != "scnr") || binary.BigEndian.Uint32(profile[64:68]) > 3 {
		return nil, ErrImage
	}
	gray := source.ColorModel() == color.GrayModel || source.ColorModel() == color.Gray16Model
	space := "RGB "
	required := []string{"rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC", "wtpt"}
	if gray {
		space = "GRAY"
		required = []string{"kTRC", "wtpt"}
	}
	if string(profile[16:20]) != space {
		return nil, ErrImage
	}
	count := uint64(binary.BigEndian.Uint32(profile[128:132]))
	tableEnd := uint64(132) + count*12
	if count > maxColorProfileTags || tableEnd > uint64(len(profile)) { // Bound table bookkeeping independently of profile bytes.
		return nil, ErrImage
	}
	tags := make(map[string][]byte)
	seen := make(map[string]bool)
	for index := uint64(0); index < count; index++ {
		entry := profile[132+index*12 : 132+(index+1)*12]
		name := string(entry[:4])
		// ICC tag records hold a four-byte signature, offset and size.
		// Offsets are absolute; payloads can be shared by multiple tags.
		offset, size := uint64(binary.BigEndian.Uint32(entry[4:8])), uint64(binary.BigEndian.Uint32(entry[8:12]))
		if seen[name] || offset < tableEnd || offset%4 != 0 || size < 8 || offset+size > uint64(len(profile)) {
			return nil, ErrImage
		}
		seen[name] = true
		switch name {
		case "A2B0", "A2B1", "A2B2", "B2A0", "B2A1", "B2A2", "D2B0", "D2B1", "D2B2", "D2B3", "B2D0", "B2D1", "B2D2", "B2D3", "cicp":
			return nil, ErrImage
		case "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC", "kTRC", "wtpt", "bkpt", "chad", "lumi":
			payload := profile[offset : offset+size]
			if !validNumericColorTag(name, payload) {
				return nil, ErrImage
			}
			tags[name] = payload
		}
	}
	for _, name := range required {
		if tags[name] == nil {
			return nil, ErrImage
		}
	}
	// Fixed identifiers are required by the ICC format but carry no source names,
	// dates or device identifiers. Keep only header fields that affect decoding.
	header := make([]byte, 128)
	copy(header[8:10], profile[8:10]) // Major/minor version; its reserved bytes remain zero.
	copy(header[12:24], profile[12:24])
	copy(header[36:40], "acsp")
	copy(header[64:80], profile[64:80])
	binary.BigEndian.PutUint16(header[24:26], 2000)
	binary.BigEndian.PutUint16(header[26:28], 1)
	binary.BigEndian.PutUint16(header[28:30], 1)
	tags["desc"] = fixedProfileText(major, "desc", "Thoughts background")
	tags["cprt"] = fixedProfileText(major, "cprt", "Source color transform")
	// Stable ordering makes the allowlist and retained data explicit.
	names := []string{"desc", "cprt", "wtpt", "bkpt", "chad", "lumi"}
	if gray {
		names = append(names, "kTRC")
	} else {
		names = append(names, "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC")
	}
	retained := make([]string, 0, len(names))
	for _, name := range names {
		if tags[name] != nil {
			retained = append(retained, name)
		}
	}
	output := append(header, make([]byte, 4+12*len(retained))...)
	binary.BigEndian.PutUint32(output[128:132], uint32(len(retained)))
	for index, name := range retained {
		payload := tags[name]
		entry := output[132+index*12 : 144+index*12]
		copy(entry, name)
		binary.BigEndian.PutUint32(entry[4:8], uint32(len(output)))
		binary.BigEndian.PutUint32(entry[8:12], uint32(len(payload)))
		output = append(output, payload...)
		for len(output)%4 != 0 {
			output = append(output, 0)
		}
	}
	binary.BigEndian.PutUint32(output[:4], uint32(len(output)))
	if major == 4 {
		// Flags, rendering intent and ID are zero during the ICC profile-ID digest.
		digestInput := bytes.Clone(output)
		clear(digestInput[64:68])
		digest := md5.Sum(digestInput)
		copy(output[84:100], digest[:])
	}
	return output, nil
}

func validNumericColorTag(name string, payload []byte) bool {
	if len(payload) < 8 || !bytes.Equal(payload[4:8], []byte{0, 0, 0, 0}) {
		return false
	}
	switch name {
	case "rXYZ", "gXYZ", "bXYZ", "wtpt", "bkpt", "lumi":
		return len(payload) == 20 && string(payload[:4]) == "XYZ "
	case "chad":
		return len(payload) == 44 && string(payload[:4]) == "sf32"
	default:
		if len(payload) < 12 {
			return false
		}
		switch string(payload[:4]) {
		case "curv":
			count := uint64(binary.BigEndian.Uint32(payload[8:12]))
			return uint64(len(payload)) == 12+2*count
		case "para":
			function := binary.BigEndian.Uint16(payload[8:10])
			parameters := [5]int{1, 3, 4, 5, 7}
			return function < 5 && payload[10] == 0 && payload[11] == 0 && len(payload) == 12+parameters[function]*4 && int32(binary.BigEndian.Uint32(payload[12:16])) > 0
		}
	}
	return false
}

func fixedProfileText(major byte, kind, text string) []byte {
	if major == 4 {
		payload := make([]byte, 28+2*len(text))
		copy(payload, "mluc")
		binary.BigEndian.PutUint32(payload[8:12], 1)
		binary.BigEndian.PutUint32(payload[12:16], 12)
		copy(payload[16:20], "enUS")
		binary.BigEndian.PutUint32(payload[20:24], uint32(2*len(text)))
		binary.BigEndian.PutUint32(payload[24:28], 28)
		for index := range text {
			payload[29+index*2] = text[index]
		}
		return payload
	}
	if kind == "desc" {
		payload := make([]byte, 12+len(text)+1+78)
		copy(payload, "desc")
		binary.BigEndian.PutUint32(payload[8:12], uint32(len(text)+1))
		copy(payload[12:], text)
		return payload
	}
	payload := make([]byte, 8+len(text)+1)
	copy(payload, "text")
	copy(payload[8:], text)
	return payload
}
