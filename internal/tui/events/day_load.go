package events

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// dayLoad owns one requested day's reads and feedback. Accepted timeline data
// stays on Model until the matching entry list establishes the displayed day.
type dayLoad struct {
	ctx            context.Context
	cancelRead     context.CancelFunc
	cancelFeedback context.CancelFunc
	generation     uint64
	pendingDay     time.Time
	pendingCounts  eventCountsMsg
	entriesPending bool
	countsPending  bool
	latestPending  bool
	retainedBody   string
	showStatus     bool
	countErr       error
	latestErr      error
}

func (l *dayLoad) invalidate() {
	if l.cancelRead != nil {
		l.cancelRead()
	}
	// Cancellation saves work; the generation still rejects already queued replies.
	l.generation++
}

func (l dayLoad) owns(generation uint64) bool { return generation == l.generation }

func (l *dayLoad) begin(parent context.Context, day time.Time, owner *int) tea.Cmd {
	l.invalidate()
	l.ctx, l.cancelRead = context.WithCancel(parent)
	feedbackCtx, cancel := context.WithCancel(l.ctx)
	l.cancelFeedback = cancel
	l.entriesPending, l.countsPending, l.latestPending = true, true, false
	l.pendingDay, l.pendingCounts, l.showStatus = day, eventCountsMsg{}, false
	generation := l.generation
	return func() tea.Msg {
		// Delay only feedback, never the reads or application of their results.
		if feedbackCtx.Err() != nil {
			return nil
		}
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-feedbackCtx.Done():
			return nil
		case <-timer.C:
			return loadDelayedMsg{owner, generation}
		}
	}
}

func (l *dayLoad) delayFeedback(generation uint64) {
	if l.owns(generation) && (l.entriesPending || l.countsPending) {
		l.showStatus = true
	}
}

func (l *dayLoad) finishEntries() {
	l.entriesPending = false
	l.retainedBody = ""
	l.finishFeedback()
}

func (l *dayLoad) finishCounts(result eventCountsMsg) {
	l.countsPending = false
	l.pendingCounts = result
	l.finishFeedback()
}

func (l *dayLoad) finishFeedback() {
	if !l.entriesPending && !l.countsPending {
		l.cancelFeedback()
		l.showStatus = false
	}
}
