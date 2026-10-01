package browserterm

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
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
	case 'p':
		if len(body) > maxPasteBytes {
			return nil, errInput
		}
		return []tea.Msg{tea.PasteMsg{Content: string(body)}}, nil
	case '2':
		var size struct{ Cols, Rows int }
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
