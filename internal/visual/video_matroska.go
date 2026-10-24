package visual

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
)

type videoSourceContainer uint8

const (
	sourceMOV videoSourceContainer = iota
	sourceMatroska
	maxMatroskaHeaderBytes   = 4096
	maxMatroskaMetadataBytes = 32 << 20
	// A four-hour 60 FPS movie can contain 864,000 blocks. Known-size clusters
	// are skipped in one seek; this also bounds headers in streaming clusters.
	maxMatroskaElements = 2_000_000
	// A selected packet normally yields one picture; admit bounded codec
	// superframes without allowing an arbitrary origin-candidate list.
	maxVideoOriginFrames = 16
)

// Wire identifiers from the Matroska schema. Only these structural declarations
// are interpreted; other payloads remain opaque to the application.
const (
	matroskaEBMLHeader          = 0x1a45dfa3
	matroskaDocType             = 0x4282
	matroskaSegment             = 0x18538067
	matroskaCluster             = 0x1f43b675
	matroskaVoid                = 0xec
	matroskaInfo                = 0x1549a966
	matroskaTracks              = 0x1654ae6b
	matroskaChapters            = 0x1043a770
	matroskaSeekHead            = 0x114d9b74
	matroskaCues                = 0x1c53bb6b
	matroskaAttachments         = 0x1941a469
	matroskaTags                = 0x1254c367
	matroskaPreviousSegment     = 0x3cb923
	matroskaNextSegment         = 0x3eb923
	matroskaPreviousFilename    = 0x3c83ab
	matroskaNextFilename        = 0x3e83bb
	matroskaChapterSegment      = 0x6e67
	matroskaContentEncryption   = 0x5035
	matroskaContentEncodingType = 0x5033
	matroskaTrackEntry          = 0xae
	matroskaContentEncodings    = 0x6d80
	matroskaContentEncoding     = 0x6240
	matroskaEditionEntry        = 0x45b9
	matroskaChapterAtom         = 0xb6
)

func (container videoSourceContainer) inputArgs() []string {
	if container == sourceMatroska {
		return []string{"-f", "matroska"}
	}
	return []string{"-f", "mov", "-enable_drefs", "0", "-export_all", "0"}
}

func identifyVideoContainer(ctx context.Context, input io.ReaderAt, size int64) (videoSourceContainer, error) {
	var signature [4]byte
	if size < 24 || size > MaxVideoSourceBytes {
		return 0, ErrVideo
	}
	if _, err := input.ReadAt(signature[:], 0); err != nil {
		return 0, ErrVideo
	}
	if binary.BigEndian.Uint32(signature[:]) != matroskaEBMLHeader {
		return sourceMOV, validateVideoContainer(input, size)
	}
	return sourceMatroska, inspectMatroska(ctx, input, size)
}

// This is a structural preflight, not a demuxer: skip packet bodies, fonts and
// tags by their declared sizes. Only DocType, ownership-independent reference
// declarations and track protection are interpreted. FFmpeg validates media.
// Its demuxer loads attachments before stream selection, so a JSON output cap
// alone cannot bound attachment/metadata allocation.
type matroskaInspector struct {
	ctx      context.Context
	input    io.ReaderAt
	elements int
	metadata int64
	tracks   int
}

type matroskaElement struct {
	id           uint64
	payload, end int64
	unknown      bool
}

func (inspection *matroskaInspector) element(offset, limit int64) (matroskaElement, error) {
	if err := inspection.ctx.Err(); err != nil {
		return matroskaElement{}, err
	}
	inspection.elements++
	if inspection.elements > maxMatroskaElements || offset >= limit {
		return matroskaElement{}, ErrVideo
	}
	// EBML's first set bit encodes a 1–8-byte length. Element IDs retain
	// that bit; sizes omit it, with all value bits set meaning unknown length.
	readNumber := func(position int64, maximum int, identifier bool) (uint64, int64, bool, error) {
		var body [8]byte
		if position >= limit {
			return 0, 0, false, ErrVideo
		}
		if _, err := inspection.input.ReadAt(body[:1], position); err != nil || body[0] == 0 {
			return 0, 0, false, ErrVideo
		}
		length, mask := 1, byte(0x80)
		for body[0]&mask == 0 {
			length++
			mask >>= 1
		}
		if length > maximum || int64(length) > limit-position {
			return 0, 0, false, ErrVideo
		}
		if _, err := inspection.input.ReadAt(body[:length], position); err != nil {
			return 0, 0, false, ErrVideo
		}
		value := uint64(body[0])
		if !identifier {
			value &= uint64(mask - 1)
		}
		for _, next := range body[1:length] {
			value = value<<8 | uint64(next)
		}
		unknown := !identifier && value == (uint64(1)<<uint(7*length))-1
		return value, int64(length), unknown, nil
	}
	id, idBytes, _, err := readNumber(offset, 4, true)
	if err != nil {
		return matroskaElement{}, err
	}
	size, sizeBytes, unknown, err := readNumber(offset+idBytes, 8, false)
	if err != nil {
		return matroskaElement{}, err
	}
	payload := offset + idBytes + sizeBytes
	if unknown {
		return matroskaElement{id, payload, limit, true}, nil
	}
	if size > uint64(limit-payload) {
		return matroskaElement{}, ErrVideo
	}
	return matroskaElement{id, payload, payload + int64(size), false}, nil
}

func inspectMatroska(ctx context.Context, input io.ReaderAt, size int64) error {
	inspection := matroskaInspector{ctx: ctx, input: input}
	header, err := inspection.element(0, size)
	if err != nil {
		return err
	}
	if header.id != matroskaEBMLHeader || header.unknown || header.end > maxMatroskaHeaderBytes {
		return ErrVideoContainer
	}
	documentType := false
	for offset := header.payload; offset < header.end; {
		item, err := inspection.element(offset, header.end)
		if err != nil {
			return err
		}
		if item.unknown {
			return ErrVideoContainer
		}
		if item.id == matroskaDocType {
			var name [8]byte
			if item.end-item.payload != int64(len(name)) {
				return ErrVideoContainer
			}
			if _, err := input.ReadAt(name[:], item.payload); err != nil || string(name[:]) != "matroska" {
				return ErrVideoContainer
			}
			documentType = true
		}
		offset = item.end
	}
	if !documentType {
		return ErrVideoContainer
	}
	segment, err := inspection.element(header.end, size)
	if err != nil {
		return err
	}
	if segment.id != matroskaSegment || segment.end != size {
		return ErrVideoContainer
	}
	var info, tracks, cluster bool
	for offset := segment.payload; offset < segment.end; {
		item, err := inspection.element(offset, segment.end)
		if err != nil {
			return err
		}
		if item.unknown && item.id != matroskaCluster {
			return ErrVideoContainer
		}
		switch item.id {
		case matroskaCluster: // Cluster holds packets; never inspect compressed payloads.
			cluster = true
			if item.unknown {
				offset, err = inspection.streamingCluster(item.payload, item.end)
				if err != nil {
					return err
				}
				continue
			}
		case matroskaVoid: // Void padding is not retained or allocated by this preflight.
		default:
			if item.unknown {
				return ErrVideoContainer
			}
			inspection.metadata += item.end - offset
			if inspection.metadata > maxMatroskaMetadataBytes {
				return ErrVideoContainer
			}
			if item.id == matroskaInfo {
				info = true
			}
			if item.id == matroskaTracks {
				tracks = true
			}
			if item.id == matroskaInfo || item.id == matroskaTracks || item.id == matroskaChapters {
				if err := inspection.declarations(item.payload, item.end, 0); err != nil {
					return err
				}
			}
		}
		offset = item.end
	}
	if !info || !tracks || !cluster {
		return ErrVideoContainer
	}
	return nil
}

func (inspection *matroskaInspector) streamingCluster(start, end int64) (int64, error) {
	for offset := start; offset < end; {
		item, err := inspection.element(offset, end)
		if err != nil {
			return 0, err
		}
		// Level-one elements end an unknown-sized cluster. Their contents are still
		// subject to the ordinary metadata budget in the outer loop.
		switch item.id {
		case matroskaCluster, matroskaInfo, matroskaTracks, matroskaChapters, matroskaSeekHead, matroskaCues, matroskaAttachments, matroskaTags:
			return offset, nil
		}
		if item.unknown {
			return 0, ErrVideoContainer
		}
		offset = item.end
	}
	return end, nil
}

func (inspection *matroskaInspector) declarations(start, end int64, depth int) error {
	if depth > 8 {
		return ErrVideoContainer
	}
	for offset := start; offset < end; {
		item, err := inspection.element(offset, end)
		if err != nil {
			return err
		}
		if item.unknown {
			return ErrVideoContainer
		}
		switch item.id {
		case matroskaPreviousSegment, matroskaNextSegment, matroskaPreviousFilename, matroskaNextFilename, matroskaChapterSegment:
			// Segment links and chapter references must not turn one upload into a
			// playlist requiring other local files. Values are never read or logged.
			return ErrVideoContainer
		case matroskaContentEncryption: // ContentEncryption, regardless of key ID or algorithm.
			return ErrVideoProtected
		case matroskaContentEncodingType: // ContentEncodingType: 1 denotes encryption.
			if item.end-item.payload < 1 || item.end-item.payload > 8 {
				return ErrVideoContainer
			}
			var value [8]byte
			if _, err := inspection.input.ReadAt(value[8-(item.end-item.payload):], item.payload); err != nil {
				return ErrVideoContainer
			}
			if binary.BigEndian.Uint64(value[:]) != 0 {
				return ErrVideoProtected
			}
		case matroskaTrackEntry:
			inspection.tracks++
			if inspection.tracks > maxVideoStreams {
				return ErrVideoContainer
			}
			if err := inspection.declarations(item.payload, item.end, depth+1); err != nil {
				return err
			}
		case matroskaContentEncodings, matroskaContentEncoding, matroskaEditionEntry, matroskaChapterAtom:
			if err := inspection.declarations(item.payload, item.end, depth+1); err != nil {
				return err
			}
		}
		offset = item.end
	}
	return nil
}

// Validate technical properties before admitting cut controls. MOV sample tables
// already provide its timing; Matroska needs a bounded selected-frame check when
// earlier audio makes FFprobe fill unknown video timing from container values.
func validateVideoSource(ctx context.Context, tool, path string, container videoSourceContainer) (videoProbe, error) {
	info, err := probeVideoSource(ctx, tool, path, container)
	if err != nil {
		return videoProbe{}, err
	}
	if container == sourceMatroska {
		// Duration may be the guessed field. All codec/color/geometry checks run
		// first, so a timing-only failure can safely proceed to one selected frame.
		if err := info.validate(false); err != nil && !errors.Is(err, ErrVideoDuration) {
			return videoProbe{}, err
		}
		if err := probeMatroskaVideoOrigin(ctx, tool, path, &info); err != nil {
			return videoProbe{}, err
		}
	}
	if err := info.validate(false); err != nil {
		return videoProbe{}, err
	}
	return info, nil
}

func probeMatroskaVideoOrigin(ctx context.Context, tool, path string, info *videoProbe) error {
	stream, ok := info.video()
	if !ok {
		return ErrVideoTracks
	}
	ctx, cancel := context.WithTimeout(ctx, MaxVideoFrameTime)
	defer cancel()
	var output videoProbeBuffer
	// #1 counts selected video packets. Drain that packet's decoder to obtain
	// display PTS, rather than trusting a container-derived guessed start_time.
	// No audio/subtitles are rendered, no pixels or source tags enter JSON, and
	// the child is joined within this short owner-bound deadline.
	args := []string{
		"-v", "error", "-max_alloc", strconv.Itoa(maxVideoAllocation),
		"-protocol_whitelist", "file", "-f", "matroska", "-threads", "1",
		"-max_pixels", strconv.Itoa(MaxVideoPixels), "-max_streams", strconv.Itoa(maxVideoStreams),
		"-probesize", strconv.Itoa(maxVideoProbeInputBytes),
		"-analyzeduration", strconv.FormatInt(maxVideoProbeAnalyzeTime.Microseconds(), 10),
		"-max_probe_packets", strconv.Itoa(maxVideoProbePackets),
		"-select_streams", strconv.Itoa(stream.Index), "-read_intervals", "%+#1",
		"-show_frames", "-show_entries", "frame=best_effort_timestamp_time:frame_side_data=",
		"-of", "json", path,
	}
	if err := runVideoTool(ctx, tool, &output, args...); err != nil {
		return err
	}
	var result struct {
		Frames []struct {
			Timestamp string `json:"best_effort_timestamp_time"`
		} `json:"frames"`
	}
	if json.Unmarshal(output.Bytes(), &result) != nil || len(result.Frames) == 0 || len(result.Frames) > maxVideoOriginFrames {
		return ErrVideoDuration
	}
	origin := math.Inf(1)
	for _, frame := range result.Frames {
		timestamp, err := strconv.ParseFloat(frame.Timestamp, 64)
		if err != nil || math.IsNaN(timestamp) || math.IsInf(timestamp, 0) {
			return ErrVideoDuration
		}
		origin = math.Min(origin, timestamp)
	}
	for index := range info.Streams {
		selected := &info.Streams[index]
		if selected.Index != stream.Index {
			continue
		}
		previous, err := strconv.ParseFloat(selected.StartTime, 64)
		if err != nil || math.Abs(previous-origin) > 0.000001 {
			// A duration inferred from the old container origin is no longer valid.
			selected.Duration = ""
		}
		selected.StartTime = strconv.FormatFloat(origin, 'f', 6, 64)
		return nil
	}
	return ErrVideoTracks
}
