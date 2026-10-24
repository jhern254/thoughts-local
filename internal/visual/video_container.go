package visual

import (
	"encoding/binary"
	"io"
)

// MOV/MP4 data references can name other files even with networking disabled.
// Inspect data references and supported display declarations before the MOV demuxer;
// accept self-contained url entries, never locations, compressed/fragmented
// movies or QuickTime reference movies. This is not a filesystem sandbox.
func validateVideoContainer(input io.ReaderAt, size int64) error {
	return inspectVideoContainer(input, size, false)
}

func inspectVideoContainer(input io.ReaderAt, size int64, prepared bool) error {
	// Long-source sample tables grow with frame count. Prepared loops retain
	// their smaller metadata bound; neither path buffers the movie index.
	indexLimit := int64(32 << 20)
	if prepared {
		indexLimit = 8 << 20
	}
	boxes := 0
	tracks := 0
	var movie, media, fileType bool
	var inspect func(int64, int64, int) error
	inspect = func(start, end int64, depth int) error {
		if depth > 8 {
			return ErrVideo
		}
		for offset := start; offset < end; {
			boxes++
			if boxes > 4096 || end-offset < 8 {
				return ErrVideo
			}
			var header [16]byte
			if _, err := input.ReadAt(header[:8], offset); err != nil {
				return ErrVideo
			}
			length, headerSize := int64(binary.BigEndian.Uint32(header[:4])), int64(8)
			if length == 1 {
				if end-offset < 16 {
					return ErrVideo
				}
				if _, err := input.ReadAt(header[8:], offset+8); err != nil {
					return ErrVideo
				}
				extended := binary.BigEndian.Uint64(header[8:])
				if extended > uint64(end-offset) {
					return ErrVideo
				}
				length, headerSize = int64(extended), 16
			} else if length == 0 {
				length = end - offset
			}
			if length < headerSize || length > end-offset {
				return ErrVideo
			}
			kind := string(header[4:8])
			payload, limit := offset+headerSize, offset+length
			switch kind {
			case "ftyp":
				fileType = depth == 0
			case "mdat":
				if prepared && depth == 0 && !movie {
					return ErrVideo
				}
				media = depth == 0
			case "moov", "trak", "mdia", "minf", "dinf", "stbl":
				if kind == "trak" {
					tracks++
					if tracks > maxVideoStreams {
						return ErrVideo
					}
				}
				if kind == "moov" {
					if depth != 0 || movie || length > indexLimit {
						return ErrVideo
					}
					movie = true
				}
				if err := inspect(payload, limit, depth+1); err != nil {
					return err
				}
			case "stsd":
				// FullBox fields plus sample count precede sample entries. H.264
				// visual entries have a fixed 78-byte header before their child boxes.
				if limit-payload < 8 {
					return ErrVideo
				}
				var fields [8]byte
				if _, err := input.ReadAt(fields[:], payload); err != nil {
					return ErrVideo
				}
				if binary.BigEndian.Uint32(fields[:4]) != 0 || binary.BigEndian.Uint32(fields[4:]) > 16 {
					return ErrVideo
				}
				if err := inspect(payload+8, limit, depth+1); err != nil {
					return err
				}
			case "avc1":
				if limit-payload < 78 {
					return ErrVideo
				}
				if err := inspect(payload+78, limit, depth+1); err != nil {
					return err
				}
			case "colr":
				// nclx/nclc declare primaries, transfer and YCbCr matrix. ICC-based
				// video profiles are outside this first SDR path, never silently stripped.
				if limit-payload != 10 && limit-payload != 11 {
					return ErrVideoColor
				}
				var color [11]byte
				if _, err := input.ReadAt(color[:limit-payload], payload); err != nil {
					return ErrVideoColor
				}
				kind := string(color[:4])
				if (kind != "nclx" || limit-payload != 11) && (kind != "nclc" || limit-payload != 10) {
					return ErrVideoColor
				}
				for offset := 4; offset < 10; offset += 2 {
					if binary.BigEndian.Uint16(color[offset:offset+2]) != 1 {
						return ErrVideoColor
					}
				}
			case "dref":
				// FullBox version/flags and count precede size/type entries.
				if limit-payload < 8 {
					return ErrVideo
				}
				var fields [8]byte
				if _, err := input.ReadAt(fields[:], payload); err != nil {
					return ErrVideo
				}
				if binary.BigEndian.Uint32(fields[:4]) != 0 {
					return ErrVideo
				}
				count := binary.BigEndian.Uint32(fields[4:])
				if count == 0 || count > 16 || limit-payload != 8+int64(count)*12 {
					return ErrVideo
				}
				for entry := uint32(0); entry < count; entry++ {
					var reference [12]byte
					if _, err := input.ReadAt(reference[:], payload+8+int64(entry)*12); err != nil {
						return ErrVideo
					}
					if binary.BigEndian.Uint32(reference[:4]) != 12 || string(reference[4:8]) != "url " || binary.BigEndian.Uint32(reference[8:]) != 1 {
						return ErrVideo
					}
				}
			case "cmov", "rmra", "rmda", "moof", "sinf", "encv", "clap":
				return ErrVideo
			}
			offset = limit
		}
		return nil
	}
	if size < 24 || size > MaxVideoSourceBytes {
		return ErrVideo
	}
	if err := inspect(0, size, 0); err != nil {
		return err
	}
	if !fileType || !movie || !media {
		return ErrVideo
	}
	return nil
}
