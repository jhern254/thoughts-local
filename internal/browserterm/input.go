package browserterm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

const (
	// Frame bounds limit transport/parser work; the TUI still validates authored
	// text. Geometry bounds match the browser client and bound terminal rendering.
	maxPasteBytes = 1 << 20
	maxKeyBytes   = 4096
	maxCols       = 512
	maxRows       = 128
)

var errInput = errors.New("invalid browser terminal input")

// Each xterm onData callback is one complete input frame. Decode it with the
// same decoder Bubble Tea uses, without retaining unfinished terminal sequences
// across frames. Paste has its own frame so neither xterm nor the terminal input
// scanner can normalize or discard characters before Thoughts validates them.
func inputMessages(frame []byte) ([]tea.Msg, error) {
	if len(frame) == 0 || !utf8.Valid(frame) {
		return nil, errInput
	}
	body := frame[1:]
	switch frame[0] {
	case 'v':
		var action thoughts.VoiceAction
		// Voice frames contain only small control metadata, so cap JSON at 256 bytes.
		// 9007199254740991 is JavaScript's Number.MAX_SAFE_INTEGER (2^53 - 1):
		// larger IDs can round in the browser and break draft/recording ownership.
		if len(body) > 256 {
			return nil, errInput
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&action) != nil || decoder.Decode(new(any)) != io.EOF || action.DraftID > 9007199254740991 || action.RecordingID > 9007199254740991 {
			return nil, errInput
		}
		// Open has no existing draft; later controls must identify one. The model
		// checks that both IDs still belong to its current recording attempt.
		switch action.Action {
		case "open":
			if action.DraftID != 0 || action.RecordingID != 0 {
				return nil, errInput
			}
		case "start", "stop", "recording", "stopped", "denied", "unavailable", "failed":
			if action.DraftID == 0 {
				return nil, errInput
			}
		default:
			return nil, errInput
		}
		return []tea.Msg{action}, nil
	case 'p':
		if len(body) > maxPasteBytes {
			return nil, errInput
		}
		return []tea.Msg{tea.PasteMsg{Content: string(body)}}, nil
	case '2':
		var size struct{ Cols, Rows int }
		// Resize JSON needs only two integers. Cap it at 128 bytes before decoding,
		// then require positive dimensions within the shared client/server bounds.
		if len(body) > 128 || json.Unmarshal(body, &size) != nil || size.Cols < 1 || size.Cols > maxCols || size.Rows < 1 || size.Rows > maxRows {
			return nil, errInput
		}
		return []tea.Msg{tea.WindowSizeMsg{Width: size.Cols, Height: size.Rows}}, nil
	case '0':
		if len(body) > maxKeyBytes {
			return nil, errInput
		}
		var decoder uv.EventDecoder
		var messages []tea.Msg
		for len(body) > 0 {
			n, event := decoder.Decode(body)
			if n <= 0 || n > len(body) {
				return nil, errInput
			}
			body = body[n:]
			var msg tea.Msg
			switch event := event.(type) {
			case uv.KeyPressEvent:
				key := tea.KeyPressMsg(event)
				// Browser paste arrives in a 'p' frame. Ignore these terminal shortcuts
				// so they cannot trigger a clipboard read on the native server's device.
				if key.String() == "ctrl+v" || key.String() == "shift+insert" {
					continue
				}
				msg = key
			case uv.MouseClickEvent:
				msg = tea.MouseClickMsg(event)
			case uv.MouseReleaseEvent:
				msg = tea.MouseReleaseMsg(event)
			case uv.MouseMotionEvent:
				msg = tea.MouseMotionMsg(event)
			case uv.MouseWheelEvent:
				msg = tea.MouseWheelMsg(event)
			case uv.FocusEvent:
				msg = tea.FocusMsg(event)
			case uv.BlurEvent:
				msg = tea.BlurMsg(event)
			case uv.CursorPositionEvent:
				msg = tea.CursorPositionMsg(event)
			case uv.ModeReportEvent:
				msg = tea.ModeReportMsg(event)
			case uv.PrimaryDeviceAttributesEvent, uv.SecondaryDeviceAttributesEvent:
				continue
			default:
				return nil, errInput
			}
			messages = append(messages, msg)
		}
		return messages, nil
	default:
		return nil, errInput
	}
}
