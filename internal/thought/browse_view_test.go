package thought

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestThoughtService_BrowseView(t *testing.T) {
	t.Run("passes scoped cursor and preserves results and original errors", func(t *testing.T) {
		query := data.ThoughtViewRequest{Cursor: &data.ThoughtCursor{ObservedAt: time.Unix(300, 0), CreatedAt: time.Unix(100, 0), ThoughtID: 7}, Direction: data.ThoughtsNewer}
		view := data.ThoughtView{Items: []data.ThoughtSummary{{ThoughtID: 8}}, More: true}
		for _, wantErr := range []error{nil, errors.New("underlying failure")} {
			ctx := t.Context()
			service := NewService(&thoughtServiceStoreStub{browseView: func(got context.Context, userID string, request data.ThoughtViewRequest) (data.ThoughtView, error) {
				if got != ctx || userID != "u" || !reflect.DeepEqual(request, query) {
					t.Fatal("browse lost context, owner or cursor")
				}
				return view, wantErr
			}})
			got, err := service.BrowseView(ctx, "u", query)
			if !reflect.DeepEqual(got, view) || err != wantErr {
				t.Fatal("browse lost result or original error")
			}
		}
	})
	t.Run("rejects ambiguous direction before persistence", func(t *testing.T) {
		service := NewService(&thoughtServiceStoreStub{browseView: func(context.Context, string, data.ThoughtViewRequest) (data.ThoughtView, error) {
			t.Fatal("invalid request reached store")
			return data.ThoughtView{}, nil
		}})
		for _, query := range []data.ThoughtViewRequest{{Direction: data.ThoughtDirection(99)}, {Direction: data.ThoughtsNewer}} {
			_, err := service.BrowseView(t.Context(), "u", query)
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("got %v, want validation", err)
			}
		}
	})
}
