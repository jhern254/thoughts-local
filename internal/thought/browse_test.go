package thought

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestThoughtService_Browse(t *testing.T) {
	t.Run("passes scoped cursor and preserves results and original errors", func(t *testing.T) {
		query := data.ThoughtPageRequest{Cursor: &data.ThoughtCursor{ObservedAt: time.Unix(300, 0), CreatedAt: time.Unix(100, 0), ThoughtID: 7}, Direction: data.ThoughtsNewer}
		page := data.ThoughtPage{Items: []data.ThoughtSummary{{ThoughtID: 8}}, More: true}
		for _, wantErr := range []error{nil, errors.New("underlying failure")} {
			ctx := t.Context()
			service := NewService(&thoughtServiceStoreStub{browse: func(got context.Context, userID string, request data.ThoughtPageRequest) (data.ThoughtPage, error) {
				if got != ctx || userID != "u" || !reflect.DeepEqual(request, query) {
					t.Fatal("browse lost context, owner or cursor")
				}
				return page, wantErr
			}})
			got, err := service.Browse(ctx, "u", query)
			if !reflect.DeepEqual(got, page) || err != wantErr {
				t.Fatal("browse lost result or original error")
			}
		}
	})
	t.Run("rejects ambiguous direction before persistence", func(t *testing.T) {
		service := NewService(&thoughtServiceStoreStub{browse: func(context.Context, string, data.ThoughtPageRequest) (data.ThoughtPage, error) {
			t.Fatal("invalid request reached store")
			return data.ThoughtPage{}, nil
		}})
		for _, query := range []data.ThoughtPageRequest{{Direction: data.ThoughtDirection(99)}, {Direction: data.ThoughtsNewer}} {
			_, err := service.Browse(t.Context(), "u", query)
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("got %v, want validation", err)
			}
		}
	})
}
