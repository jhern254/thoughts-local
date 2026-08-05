package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

func TestPostSubjectPersistsToSQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := data.EnableSQLiteFK(db); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := data.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := ensureDevelopmentUser(context.Background(), db); err != nil {
		t.Fatalf("ensure development user: %v", err)
	}

	database, cleanDatabase := testutils.CreateTempFile(t, `[]`)
	defer cleanDatabase()
	thoughtStore, err := data.NewFileSystemThoughtStore(database)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}
	store := data.NewCompositeStore(thoughtStore, data.NewSQLiteSubjectStore(db))
	server := NewApplication(store, config{env: "development"}, zerolog.New(io.Discard))

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, newPostSubjectRequest("coding"))

	testutils.AssertStatusCode(t, response.Code, http.StatusCreated)
	var env subjectEnvelope
	if err := json.NewDecoder(response.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Subject.SubjectID == 0 {
		t.Fatal("got subject ID 0, want generated ID")
	}
	if env.Subject.UserID != "test-user" || env.Subject.SubjectName != "coding" {
		t.Fatalf("got subject %#v", env.Subject)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM subjects WHERE subject_id = ?`, env.Subject.SubjectID).Scan(&count); err != nil {
		t.Fatalf("query persisted subject: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d persisted subjects, want 1", count)
	}
}
