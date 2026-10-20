package subject

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type aliasStoreStub struct {
	listAliases     func(context.Context, string, int64) ([]data.SubjectAlias, error)
	listUserAliases func(context.Context, string) ([]data.SubjectAlias, error)
	createAlias     func(context.Context, *data.SubjectAlias) (*data.SubjectAlias, error)
	updateAlias     func(context.Context, string, int64, int64, string, time.Time) (*data.SubjectAlias, error)
	removeAlias     func(context.Context, string, int64, int64) error
	resolveName     func(context.Context, string, string) (*data.Subject, error)
}

func (s *aliasStoreStub) ListSubjectAliases(ctx context.Context, userID string, subjectID int64) ([]data.SubjectAlias, error) {
	return s.listAliases(ctx, userID, subjectID)
}
func (s *aliasStoreStub) ListUserSubjectAliases(ctx context.Context, userID string) ([]data.SubjectAlias, error) {
	return s.listUserAliases(ctx, userID)
}
func (s *aliasStoreStub) CreateSubjectAlias(ctx context.Context, alias *data.SubjectAlias) (*data.SubjectAlias, error) {
	return s.createAlias(ctx, alias)
}
func (s *aliasStoreStub) UpdateSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string, updatedAt time.Time) (*data.SubjectAlias, error) {
	return s.updateAlias(ctx, userID, subjectID, aliasID, name, updatedAt)
}
func (s *aliasStoreStub) RemoveSubjectAlias(ctx context.Context, userID string, subjectID, aliasID int64) error {
	return s.removeAlias(ctx, userID, subjectID, aliasID)
}
func (s *aliasStoreStub) ResolveSubjectName(ctx context.Context, userID, name string) (*data.Subject, error) {
	return s.resolveName(ctx, userID, name)
}

func TestSubjectService_Aliases(t *testing.T) {
	t.Run("validates create and edit before persistence with fixed feedback", func(t *testing.T) {
		service := NewService(&subjectServiceStoreStub{})
		for _, name := range []string{"", "   ", strings.Repeat("界", 256)} {
			for _, edit := range []bool{false, true} {
				var err error
				if edit {
					_, err = service.UpdateAlias(context.Background(), "owner", 1, 2, name)
				} else {
					_, err = service.CreateAlias(context.Background(), "owner", 1, name)
				}
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.PublicFields()["alias_name"] != "must be between 1 and 255 characters long" {
					t.Fatalf("validation: got %v, want fixed alias-name guidance", err)
				}
			}
		}
	})
	t.Run("preserves spelling case whitespace and Unicode", func(t *testing.T) {
		for _, name := range []string{"coding", "coDing", "  Golang  ", strings.Repeat("界", 255)} {
			store := &subjectServiceStoreStub{}
			store.createAlias = func(ctx context.Context, alias *data.SubjectAlias) (*data.SubjectAlias, error) {
				if alias.UserID != "owner" || alias.SubjectID != 7 || alias.AliasName != name || alias.CreatedAt.IsZero() || alias.UpdatedAt != alias.CreatedAt {
					t.Fatalf("created alias: got %#v, want unchanged name and initialized identity/times", alias)
				}
				return alias, nil
			}
			store.updateAlias = func(ctx context.Context, userID string, subjectID, aliasID int64, gotName string, updatedAt time.Time) (*data.SubjectAlias, error) {
				if userID != "owner" || subjectID != 7 || aliasID != 8 || gotName != name || updatedAt.IsZero() {
					t.Fatal("edit lost identity, spelling, or update time")
				}
				return &data.SubjectAlias{AliasName: gotName}, nil
			}
			service := NewService(store)
			if _, err := service.CreateAlias(context.Background(), "owner", 7, name); err != nil {
				t.Fatal(err)
			}
			if _, err := service.UpdateAlias(context.Background(), "owner", 7, 8, name); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("forwards ownership conflicts cancellation and exact lookup without rewriting errors", func(t *testing.T) {
		for _, want := range []error{data.ErrRecordNotFound, data.ErrDuplicateRecord, context.Canceled} {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			check := func(gotCtx context.Context, userID string, subjectID int64) {
				if gotCtx != ctx || userID != "owner" || subjectID != 7 {
					t.Fatal("store did not receive owner context and identity")
				}
			}
			store := &subjectServiceStoreStub{}
			store.listAliases = func(c context.Context, u string, s int64) ([]data.SubjectAlias, error) {
				check(c, u, s)
				return nil, want
			}
			store.listUserAliases = func(c context.Context, u string) ([]data.SubjectAlias, error) { check(c, u, 7); return nil, want }
			store.createAlias = func(c context.Context, a *data.SubjectAlias) (*data.SubjectAlias, error) {
				check(c, a.UserID, a.SubjectID)
				return nil, want
			}
			store.updateAlias = func(c context.Context, u string, s, a int64, n string, at time.Time) (*data.SubjectAlias, error) {
				check(c, u, s)
				return nil, want
			}
			store.removeAlias = func(c context.Context, u string, s, a int64) error { check(c, u, s); return want }
			store.resolveName = func(c context.Context, u, n string) (*data.Subject, error) {
				check(c, u, 7)
				if n != "coDing" {
					t.Fatalf("lookup name: got %q, want coDing", n)
				}
				return nil, want
			}
			service := NewService(store)
			_, err := service.ListAliases(ctx, "owner", 7)
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = service.ListAliasesForUser(ctx, "owner")
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = service.CreateAlias(ctx, "owner", 7, "Golang")
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = service.UpdateAlias(ctx, "owner", 7, 8, "Golang")
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			if err = service.RemoveAlias(ctx, "owner", 7, 8); !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = service.ResolveName(ctx, "owner", "coDing")
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
		}
	})
}
