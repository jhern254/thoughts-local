package visual

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

// FFmpeg prints only our fixed metadata key before trim/FPS filters. Counting
// these records includes seek preroll and ignores potentially malformed source
// timestamps. Exceeding the budget cancels and joins the child; a trim filter
// alone would discard excess frames while allowing decoding to continue.
// Direct writes minimize buffering; decoder/pipe read-ahead is still possible.
type videoDecodeBudget struct {
	cancel    context.CancelFunc
	maxFrames int64
	frames    int64
	line      []byte
	wantValue bool
	err       error
}

func (budget *videoDecodeBudget) Write(body []byte) (int, error) {
	if budget.err != nil {
		return 0, budget.err
	}
	for _, value := range body {
		if value != '\n' {
			if len(budget.line) >= 256 {
				return 0, budget.reject()
			}
			budget.line = append(budget.line, value)
			continue
		}
		if budget.wantValue {
			if !bytes.Equal(budget.line, []byte("thoughts.frame=1")) {
				return 0, budget.reject()
			}
		} else {
			fields := strings.Fields(string(budget.line))
			if len(fields) != 3 || !strings.HasPrefix(fields[0], "frame:") {
				return 0, budget.reject()
			}
			index, err := strconv.ParseInt(strings.TrimPrefix(fields[0], "frame:"), 10, 64)
			if err != nil || index != budget.frames || budget.frames >= budget.maxFrames {
				return 0, budget.reject()
			}
			budget.frames++
		}
		budget.wantValue = !budget.wantValue
		budget.line = budget.line[:0]
	}
	return len(body), nil
}

func (budget *videoDecodeBudget) reject() error {
	budget.err = ErrVideo
	budget.cancel()
	return budget.err
}
