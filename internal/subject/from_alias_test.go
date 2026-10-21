package subject

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func (s *subjectServiceStoreStub) CreateSubjectFromAlias(ctx context.Context, item *data.Subject, subjectID, aliasID int64) (*data.Subject, error) {
	return s.createFromAlias(ctx, item, subjectID, aliasID)
}

func TestSubjectService_CreateFromAlias(t *testing.T) {
	t.Run("validates the Subject name before removing an alias", func(t *testing.T) {
		service := NewService(&subjectServiceStoreStub{})
		for _, name := range []string{"", "   ", strings.Repeat("界", 256)} {
			_, err := service.CreateFromAlias(t.Context(), "owner", 1, 2, name)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.PublicFields()["subject_name"] == "" {
				t.Fatalf("validation: got %v, want Subject-name error", err)
			}
		}
	})
	t.Run("passes exact identity name and caller context to the transaction", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		store := &subjectServiceStoreStub{createFromAlias: func(gotCtx context.Context, item *data.Subject, subjectID, aliasID int64) (*data.Subject, error) {
			if gotCtx != ctx || item.UserID != "owner" || subjectID != 7 || aliasID != 8 || item.SubjectName != "  界 coDing  " || item.CreatedAt.IsZero() || item.CreatedAt != item.UpdatedAt {
				t.Fatalf("transaction arguments: got %+v,%d,%d, want exact identity and initialized timestamps", item, subjectID, aliasID)
			}
			return nil, data.ErrDuplicateRecord
		}}
		if _, err := NewService(store).CreateFromAlias(ctx, "owner", 7, 8, "  界 coDing  "); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("store error: got %v, want duplicate", err)
		}
	})
}
