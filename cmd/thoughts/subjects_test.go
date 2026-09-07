package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

type subjectServiceStub struct {
	create       func(context.Context, string, string) (*data.Subject, error)
	get          func(context.Context, string, int64) (*data.Subject, error)
	list         func(context.Context, string) ([]data.Subject, error)
	update       func(context.Context, string, int64, string) (*data.Subject, error)
	delete       func(context.Context, string, int64) error
	createCalled bool
	getCalled    bool
	listCalled   bool
	updateCalled bool
	deleteCalled bool
}

func TestSubjectsCommand_Logging(t *testing.T) {
	t.Run("logs mutation ID without authored content", func(t *testing.T) {
		var logs, output bytes.Buffer
		service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 7, SubjectName: "private subject"}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &output, logger: newTestLogger(t, &logs)}

		if err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "create", "private subject"}); err != nil {
			t.Fatal(err)
		}

		got := logs.String()
		if output.String() != "Created subject 7: private subject\n" {
			t.Fatalf("changed command output: %q", &output)
		}
		for _, want := range []string{"subject created", `"subject_id":7`} {
			if !strings.Contains(got, want) {
				t.Fatalf("logs %q do not contain %q", got, want)
			}
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 6 || event["application"] != "test" || event["level"] != "info" || event["message"] != "subject created" || event["subject_id"] != float64(7) || event["caller"] == nil || event["time"] == nil {
			t.Fatalf("unexpected event: %v", event)
		}
		if strings.Contains(got, "private subject") {
			t.Fatalf("logs %q contain authored content", got)
		}
	})

	for _, operation := range []string{"create", "get", "list", "update", "delete"} {
		for _, tt := range []struct {
			name   string
			err    error
			logged bool
		}{
			{name: "unexpected failure", err: errors.New("PRIVATE-ERROR-MARKER"), logged: true},
			{name: "validation", err: &subject.ValidationError{Fields: map[string]string{"subject_name": "must be provided"}}},
			{name: "not found", err: data.ErrRecordNotFound},
			{name: "duplicate", err: data.ErrDuplicateRecord},
		} {
			// Exercise classification once; other operations only need wiring coverage.
			if operation != "create" && !tt.logged {
				continue
			}
			t.Run(operation+" handles "+tt.name, func(t *testing.T) {
				var logs bytes.Buffer
				failure := fmt.Errorf("wrapped: %w", tt.err)
				service := &subjectServiceStub{
					create: func(context.Context, string, string) (*data.Subject, error) { return nil, failure },
					get:    func(context.Context, string, int64) (*data.Subject, error) { return nil, failure },
					list:   func(context.Context, string) ([]data.Subject, error) { return nil, failure },
					update: func(context.Context, string, int64, string) (*data.Subject, error) { return nil, failure },
					delete: func(context.Context, string, int64) error { return failure },
				}
				app := &application{subjects: service, userID: "user-1", out: io.Discard, logger: newTestLogger(t, &logs)}
				args := []string{"subjects", operation}
				if operation != "list" {
					args = append(args, "7")
				}
				if operation == "update" {
					args = append(args, "private subject")
				}
				if err := newSubjectsCommand(app).Run(context.Background(), args); !errors.Is(err, failure) {
					t.Fatalf("got error %v, want %v", err, failure)
				}
				if !tt.logged {
					if logs.Len() != 0 {
						t.Fatalf("unexpected logs: %s", &logs)
					}
					return
				}
				var event map[string]any
				if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if len(event) != 7 || event["operation"] != "subject_"+operation || event["level"] != "error" || event["category"] != "unexpected_failure" || event["application"] != "test" || event["message"] != "operation failed" || event["caller"] == nil || event["time"] == nil || strings.Contains(logs.String(), "PRIVATE-ERROR-MARKER") {
					t.Fatalf("incorrect failure event: %v", event)
				}
			})
		}
	}
}

func (s *subjectServiceStub) Create(ctx context.Context, userID, name string) (*data.Subject, error) {
	s.createCalled = true
	return s.create(ctx, userID, name)
}

func (s *subjectServiceStub) Get(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
	s.getCalled = true
	return s.get(ctx, userID, subjectID)
}

func (s *subjectServiceStub) List(ctx context.Context, userID string) ([]data.Subject, error) {
	s.listCalled = true
	return s.list(ctx, userID)
}

func (s *subjectServiceStub) Update(ctx context.Context, userID string, subjectID int64, name string) (*data.Subject, error) {
	s.updateCalled = true
	return s.update(ctx, userID, subjectID, name)
}

func (s *subjectServiceStub) Delete(ctx context.Context, userID string, subjectID int64) error {
	s.deleteCalled = true
	return s.delete(ctx, userID, subjectID)
}

func TestSubjectsCommand_Create(t *testing.T) {
	t.Run("creates subject", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		service := &subjectServiceStub{create: func(gotCtx context.Context, userID, name string) (*data.Subject, error) {
			if gotCtx.Value(contextKey("request")) != "test-value" || userID != "user-1" || name != "  learn Go  " {
				t.Fatalf("got context %v, user ID %q, and name %q", gotCtx, userID, name)
			}
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(ctx, []string{"subjects", "create", "  learn Go  "})

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "Created subject 7:   learn Go  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "requires subject name", args: []string{"subjects", "create"}},
		{name: "rejects extra arguments", args: []string{"subjects", "create", "coding", "extra"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) {
				return nil, errors.New("unexpected service call")
			}}
			app := &application{subjects: service, userID: "user-1", out: &bytes.Buffer{}}

			err := newSubjectsCommand(app).Run(context.Background(), tt.args)

			if err == nil {
				t.Fatal("expected argument error")
			}
			if service.createCalled {
				t.Fatal("service called with invalid arguments")
			}
		})
	}

	t.Run("returns service error", func(t *testing.T) {
		want := errors.New("create failed")
		var out bytes.Buffer
		service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) {
			return nil, want
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "create", "coding"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		service := &subjectServiceStub{create: func(_ context.Context, userID, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: errorWriter{err: want}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "create", "coding"})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

func TestSubjectsCommand_Get(t *testing.T) {
	t.Run("gets subject", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		service := &subjectServiceStub{get: func(gotCtx context.Context, userID string, subjectID int64) (*data.Subject, error) {
			if gotCtx.Value(contextKey("request")) != "test-value" || userID != "user-1" || subjectID != 7 {
				t.Fatalf("got context %v, user ID %q, and subject ID %d", gotCtx, userID, subjectID)
			}
			return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: "coding"}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(ctx, []string{"subjects", "get", "7"})

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "Subject 7: coding\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	testInvalidSubjectCommandArguments(t, "get", [][]string{
		{"subjects", "get"},
		{"subjects", "get", "7", "extra"},
		{"subjects", "get", "invalid"},
		{"subjects", "get", "0"},
		{"subjects", "get", "-1"},
	}, func(service *subjectServiceStub) bool { return service.getCalled })

	t.Run("returns service error", func(t *testing.T) {
		want := errors.New("get failed")
		var out bytes.Buffer
		service := &subjectServiceStub{get: func(context.Context, string, int64) (*data.Subject, error) {
			return nil, want
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "get", "7"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		service := &subjectServiceStub{get: func(_ context.Context, userID string, subjectID int64) (*data.Subject, error) {
			return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: "coding"}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: errorWriter{err: want}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "get", "7"})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

func TestSubjectsCommand_List(t *testing.T) {
	t.Run("lists subjects", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		service := &subjectServiceStub{list: func(gotCtx context.Context, userID string) ([]data.Subject, error) {
			if gotCtx.Value(contextKey("request")) != "test-value" || userID != "user-1" {
				t.Fatalf("got context %v and user ID %q", gotCtx, userID)
			}
			return []data.Subject{
				{SubjectID: 7, UserID: userID, SubjectName: "coding"},
				{SubjectID: 9, UserID: userID, SubjectName: "writing"},
			}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(ctx, []string{"subjects", "list"})

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "7\tcoding\n9\twriting\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	t.Run("prints nothing for empty list", func(t *testing.T) {
		var out bytes.Buffer
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return []data.Subject{}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "list"})

		if err != nil {
			t.Fatal(err)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("rejects arguments", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, errors.New("unexpected service call")
		}}
		app := &application{subjects: service, userID: "user-1", out: &bytes.Buffer{}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "list", "extra"})

		if err == nil {
			t.Fatal("expected argument error")
		}
		if service.listCalled {
			t.Fatal("service called with invalid arguments")
		}
	})

	t.Run("returns service error", func(t *testing.T) {
		want := errors.New("list failed")
		var out bytes.Buffer
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, want
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "list"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return []data.Subject{{SubjectID: 7, SubjectName: "coding"}}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: errorWriter{err: want}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "list"})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

func TestSubjectsCommand_Update(t *testing.T) {
	t.Run("updates subject", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		service := &subjectServiceStub{update: func(gotCtx context.Context, userID string, subjectID int64, name string) (*data.Subject, error) {
			if gotCtx.Value(contextKey("request")) != "test-value" || userID != "user-1" || subjectID != 7 || name != "  Go programming  " {
				t.Fatalf("got context %v, user ID %q, subject ID %d, and name %q", gotCtx, userID, subjectID, name)
			}
			return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: name}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(ctx, []string{"subjects", "update", "7", "  Go programming  "})

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "Updated subject 7:   Go programming  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	testInvalidSubjectCommandArguments(t, "update", [][]string{
		{"subjects", "update"},
		{"subjects", "update", "7"},
		{"subjects", "update", "7", "coding", "extra"},
		{"subjects", "update", "invalid", "coding"},
		{"subjects", "update", "0", "coding"},
		{"subjects", "update", "-1", "coding"},
	}, func(service *subjectServiceStub) bool { return service.updateCalled })

	t.Run("returns service error", func(t *testing.T) {
		want := errors.New("update failed")
		var out bytes.Buffer
		service := &subjectServiceStub{update: func(context.Context, string, int64, string) (*data.Subject, error) {
			return nil, want
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "update", "7", "coding"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		service := &subjectServiceStub{update: func(_ context.Context, userID string, subjectID int64, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: name}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: errorWriter{err: want}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "update", "7", "coding"})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

func TestSubjectsCommand_Delete(t *testing.T) {
	t.Run("deletes subject", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		service := &subjectServiceStub{delete: func(gotCtx context.Context, userID string, subjectID int64) error {
			if gotCtx.Value(contextKey("request")) != "test-value" || userID != "user-1" || subjectID != 7 {
				t.Fatalf("got context %v, user ID %q, and subject ID %d", gotCtx, userID, subjectID)
			}
			return nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(ctx, []string{"subjects", "delete", "7"})

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "Deleted subject 7\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	testInvalidSubjectCommandArguments(t, "delete", [][]string{
		{"subjects", "delete"},
		{"subjects", "delete", "7", "extra"},
		{"subjects", "delete", "invalid"},
		{"subjects", "delete", "0"},
		{"subjects", "delete", "-1"},
	}, func(service *subjectServiceStub) bool { return service.deleteCalled })

	t.Run("returns service error", func(t *testing.T) {
		want := errors.New("delete failed")
		var out bytes.Buffer
		service := &subjectServiceStub{delete: func(context.Context, string, int64) error {
			return want
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "delete", "7"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		service := &subjectServiceStub{delete: func(context.Context, string, int64) error { return nil }}
		app := &application{subjects: service, userID: "user-1", out: errorWriter{err: want}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "delete", "7"})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

func testInvalidSubjectCommandArguments(t *testing.T, command string, testCases [][]string, called func(*subjectServiceStub) bool) {
	t.Helper()
	for _, args := range testCases {
		t.Run("rejects "+command+" arguments "+args[len(args)-1], func(t *testing.T) {
			service := &subjectServiceStub{}
			app := &application{subjects: service, userID: "user-1", out: &bytes.Buffer{}}

			err := newSubjectsCommand(app).Run(context.Background(), args)

			if err == nil {
				t.Fatal("expected argument error")
			}
			if called(service) {
				t.Fatal("service called with invalid arguments")
			}
		})
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
