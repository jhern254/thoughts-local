//go:build integration

package integration_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func TestEventCompletionWorkflow_SQLite(t *testing.T) {
	t.Run("runtime ends and corrects an event while preserving immutable metadata", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})
		s, user := runtime.Events(), runtime.LocalUser().UserID
		item, err := s.Create(t.Context(), user, "first", eventTime(100))
		if err != nil {
			t.Fatal(err)
		}
		ended, err := s.End(t.Context(), user, item.EventID, item.Version, eventTime(200))
		if err != nil {
			t.Fatal(err)
		}
		if ended.EndedAt == nil || !ended.EndedAt.Equal(eventTime(200)) || ended.Version != 2 {
			t.Fatalf("got ended event %+v, want end 200 and version 2", ended)
		}
		end := eventTime(250)
		updated, err := s.Update(t.Context(), user, item.EventID, ended.Version, " corrected ", eventTime(50), &end)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(t.Context(), user, item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, updated) || got.Version != 3 || got.StartedAt.Unix() != 50 || got.EndedAt == nil || got.EndedAt.Unix() != 250 || got.ActivityType == nil || *got.ActivityType != "corrected" || !got.CreatedAt.Equal(item.CreatedAt) || got.UpdatedAt.Before(ended.UpdatedAt) {
			t.Fatalf("got saved event %+v, want corrected [50,250), label corrected, version 3 and retained creation time", got)
		}
		thought, err := runtime.Thoughts().Create(t.Context(), user, "inside corrected interval", nil, eventTime(225))
		if err != nil {
			t.Fatal(err)
		}
		thoughts, err := s.ListThoughts(t.Context(), user, item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if len(thoughts) != 1 || thoughts[0].ThoughtID != thought.ThoughtID {
			t.Fatalf("got thoughts %+v, want thought %d through runtime wiring", thoughts, thought.ThoughtID)
		}
	})
	for _, scenario := range []struct {
		name      string
		operation func(*event.Service, *data.Event) error
		want      error
	}{
		{"overlap correction", func(s *event.Service, e *data.Event) error {
			end := eventTime(350)
			_, err := s.Update(t.Context(), "u", e.EventID, e.Version, "changed", eventTime(100), &end)
			return err
		}, data.ErrEventOverlap},
		{"stale correction", func(s *event.Service, e *data.Event) error {
			_, err := s.Update(t.Context(), "u", e.EventID, e.Version+1, "changed", e.StartedAt, e.EndedAt)
			return err
		}, data.ErrEventVersionConflict},
		{"reopening", func(s *event.Service, e *data.Event) error {
			_, err := s.Update(t.Context(), "u", e.EventID, e.Version, "changed", e.StartedAt, nil)
			return err
		}, data.ErrEventStateConflict},
		{"ending a completed event", func(s *event.Service, e *data.Event) error {
			_, err := s.End(t.Context(), "u", e.EventID, e.Version, eventTime(250))
			return err
		}, data.ErrEventStateConflict},
		{"another user's correction", func(s *event.Service, e *data.Event) error {
			_, err := s.Update(t.Context(), "other", e.EventID, e.Version, "changed", e.StartedAt, e.EndedAt)
			return err
		}, data.ErrRecordNotFound},
		{"another user's end", func(s *event.Service, e *data.Event) error {
			_, err := s.End(t.Context(), "other", e.EventID, e.Version, eventTime(250))
			return err
		}, data.ErrRecordNotFound},
	} {
		t.Run(scenario.name+" leaves the saved event unchanged", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other")
			s := event.NewService(data.NewSQLiteEventStore(db), data.NewSQLiteThoughtStore(db))
			item, err := s.CreatePast(t.Context(), "u", "original", eventTime(100), eventTime(200))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreatePast(t.Context(), "u", "neighbor", eventTime(300), eventTime(400)); err != nil {
				t.Fatal(err)
			}
			if err := scenario.operation(s, item); !errors.Is(err, scenario.want) {
				t.Fatalf("got error %v, want %v", err, scenario.want)
			}
			got, err := s.Get(t.Context(), "u", item.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, item) {
				t.Fatalf("got saved event %+v, want unchanged %+v", got, item)
			}
		})
	}
	for _, scenario := range []struct {
		name         string
		end, version int64
		want         error
	}{
		{"ending before the start", 99, 1, data.ErrInvalidEventInterval},
		{"ending with a stale version", 200, 2, data.ErrEventVersionConflict},
	} {
		t.Run(scenario.name+" preserves the ongoing event", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			s := event.NewService(data.NewSQLiteEventStore(db), data.NewSQLiteThoughtStore(db))
			item, err := s.Create(t.Context(), "u", "", eventTime(100))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.End(t.Context(), "u", item.EventID, scenario.version, eventTime(scenario.end)); !errors.Is(err, scenario.want) {
				t.Fatalf("got error %v, want %v", err, scenario.want)
			}
			got, err := s.Get(t.Context(), "u", item.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, item) {
				t.Fatalf("got saved event %+v, want unchanged %+v", got, item)
			}
		})
	}
}

func TestEventThoughtsWorkflow_SQLite(t *testing.T) {
	t.Run("matches the whole overnight interval across subjects regardless of event links", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		s := event.NewService(data.NewSQLiteEventStore(db), data.NewSQLiteThoughtStore(db))
		item, err := s.CreatePast(t.Context(), "u", "overnight", eventTime(80000), eventTime(90000))
		if err != nil {
			t.Fatal(err)
		}
		next, err := s.CreatePast(t.Context(), "u", "next", eventTime(90000), eventTime(100000))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'u','one'),(2,'u','two')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO thoughts (thought_id,user_id,subject_id,event_id,thought,observed_at) VALUES
			(1,'u',NULL,?,'before start',79999),
			(2,'u',1,NULL,'at start',80000),
			(3,'u',2,?,'different link',85000),
			(4,'u',NULL,NULL,'same second',85000),
			(5,'u',NULL,NULL,'next day',89999),
			(6,'u',NULL,?,'at end',90000),
			(7,'other',NULL,NULL,'other user',85000),
			(8,'u',NULL,NULL,'deleted',85000)`, item.EventID, next.EventID, item.EventID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE thoughts SET deleted_at=updated_at WHERE thought_id=8`); err != nil {
			t.Fatal(err)
		}
		got, err := s.ListThoughts(t.Context(), "u", item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]int64, len(got))
		for i := range got {
			ids[i] = got[i].ThoughtID
		}
		if want := []int64{2, 3, 4, 5}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("got thought IDs %v, want %v", ids, want)
		}
		if got[1].EventID == nil || *got[1].EventID != next.EventID {
			t.Fatalf("got explicit link %v, want retained event %d", got[1].EventID, next.EventID)
		}
		got, err = s.ListThoughts(t.Context(), "u", next.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ThoughtID != 6 {
			t.Fatalf("got successor thoughts %+v, want boundary thought 6 only", got)
		}
	})
	for _, scenario := range []string{"zero duration", "other user", "deleted event", "deleted user"} {
		t.Run(scenario+" exposes no event thoughts", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other")
			s := event.NewService(data.NewSQLiteEventStore(db), data.NewSQLiteThoughtStore(db))
			end := eventTime(200)
			if scenario == "zero duration" {
				end = eventTime(100)
			}
			item, err := s.CreatePast(t.Context(), "u", "", eventTime(100), end)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO thoughts (user_id,thought,observed_at) VALUES ('u','at start',100)`); err != nil {
				t.Fatal(err)
			}
			user := "u"
			switch scenario {
			case "other user":
				user = "other"
			case "deleted event":
				if _, err := db.Exec(`UPDATE events SET deleted_at=updated_at`); err != nil {
					t.Fatal(err)
				}
			case "deleted user":
				if _, err := db.Exec(`UPDATE users SET deleted_at=updated_at WHERE user_id='u'`); err != nil {
					t.Fatal(err)
				}
			}
			got, err := s.ListThoughts(t.Context(), user, item.EventID)
			if scenario == "zero duration" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got error %v, want not found", err)
			}
			if len(got) != 0 {
				t.Fatalf("got %d thoughts, want zero", len(got))
			}
			if scenario == "deleted user" {
				got, err := data.NewSQLiteThoughtStore(db).ListThoughtsInRange(t.Context(), "u", eventTime(100), eventTime(200))
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 0 {
					t.Fatalf("got %d thoughts from inactive owner, want zero", len(got))
				}
			}
		})
	}
	t.Run("ongoing events exclude future observations", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		s := event.NewService(data.NewSQLiteEventStore(db), data.NewSQLiteThoughtStore(db))
		item, err := s.Create(t.Context(), "u", "", eventTime(100))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO thoughts (thought_id,user_id,thought,observed_at) VALUES (1,'u','past',100),(2,'u','future',?)`, time.Now().Add(24*time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
		got, err := s.ListThoughts(t.Context(), "u", item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ThoughtID != 1 {
			t.Fatalf("got thoughts %+v, want past thought 1 only", got)
		}
	})
}
