package browserterm

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"

	"github.com/jhern254/go-thoughts/internal/visual"
)

// videoEditor belongs to one WebSocket session. Opening Visual creates a fresh
// capability/context; closing it cancels admitted HTTP work before queued cleanup.
// gate protects staged files through processing, streaming and persistence.
// It is never held by Bubble Tea's synchronous Update path.
type videoEditor struct {
	ctx        context.Context
	mu         sync.Mutex
	viewCtx    context.Context
	cancelView context.CancelFunc
	id         string
	gate       chan struct{}
	// Rendering permission ends independently of the staged source/accepted loop.
	previewCtx     context.Context
	cancelPreview  context.CancelFunc
	previewRequest uint64

	cleanupErr error
	filesOwner string // Editor ID owning staged files; delayed cleanup must not remove a newer editor's files.
	source     *visual.VideoSource
	sourceID   string
	accepted   *visual.PreparedVideo // Browser-acknowledged loop eligible for Apply; not yet a saved background.
	acceptedID string
	pending    *visual.PreparedVideo // Prepared candidate awaiting acknowledgement; the accepted loop stays usable.
	pendingID  string
}

func newVideoEditor(ctx context.Context) *videoEditor {
	return &videoEditor{ctx: ctx, gate: make(chan struct{}, 1)}
}

// setOpen returns cleanup work for the existing owned Bubble Tea command path.
// Close also cleans all files, including when queued commands never start.
func (editor *videoEditor) setOpen(open bool) (string, func() error) {
	editor.mu.Lock()
	defer editor.mu.Unlock()
	if open {
		if editor.viewCtx == nil {
			editor.viewCtx, editor.cancelView = context.WithCancel(editor.ctx)
			editor.id = rand.Text()
			editor.previewCtx, editor.cancelPreview, editor.previewRequest = nil, nil, 0
		}
		return editor.id, nil
	}
	if editor.viewCtx == nil {
		return "", nil
	}
	oldID := editor.id
	editor.cancelView()
	editor.viewCtx = nil
	editor.cancelView = nil
	editor.id = ""
	return "", func() error {
		editor.gate <- struct{}{}
		defer func() { <-editor.gate }()
		if editor.filesOwner == oldID {
			err := editor.discardFiles()
			editor.cleanupErr = errors.Join(editor.cleanupErr, err)
			return err
		}
		return nil
	}
}

// Retain the revoked request identity until another render starts: late HTTP
// requests must not restart it, and Stop still needs to join its cleanup.
func (editor *videoEditor) setPreviewOperation(request uint64, preparing bool) {
	editor.mu.Lock()
	defer editor.mu.Unlock()
	if editor.viewCtx == nil || (preparing && request == editor.previewRequest && editor.previewCtx != nil) {
		return
	}
	if editor.cancelPreview != nil {
		editor.cancelPreview()
	}
	if preparing {
		editor.previewCtx, editor.cancelPreview = context.WithCancel(editor.viewCtx)
		editor.previewRequest = request
	}
}

func (editor *videoEditor) previewContext(id string, request uint64) (context.Context, bool) {
	editor.mu.Lock()
	defer editor.mu.Unlock()
	valid := id != "" && id == editor.id && editor.viewCtx != nil && editor.viewCtx.Err() == nil && request != 0 && request == editor.previewRequest && editor.previewCtx != nil
	return editor.previewCtx, valid
}

func (editor *videoEditor) context(id string) (context.Context, bool) {
	editor.mu.Lock()
	defer editor.mu.Unlock()
	return editor.viewCtx, id != "" && id == editor.id && editor.viewCtx != nil && editor.viewCtx.Err() == nil
}
func (editor *videoEditor) Close() error {
	editor.setOpen(false)
	editor.gate <- struct{}{}
	defer func() { <-editor.gate }()
	return errors.Join(editor.cleanupErr, editor.discardFiles())
}
func (editor *videoEditor) discardFiles() error {
	var errs []error
	if editor.source != nil {
		errs = append(errs, editor.source.Close())
	}
	if editor.accepted != nil {
		errs = append(errs, editor.accepted.Close())
	}
	if editor.pending != nil {
		errs = append(errs, editor.pending.Close())
	}
	editor.source = nil
	editor.accepted = nil
	editor.pending = nil
	editor.sourceID = ""
	editor.acceptedID = ""
	editor.pendingID = ""
	editor.filesOwner = ""
	return errors.Join(errs...)
}
