//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func attributionFixture(t *testing.T) (*sql.DB, *event.Service) {
	t.Helper()
	db, _ := openMigratedSQLite(t)
	seedSubjectGoals(t, db)
	if _, err := db.Exec(`
    UPDATE goals SET goal_is_active=1 WHERE goal_id=2;
    INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES
        (1,1,'owner',1), (2,1,'owner',1), (2,2,'owner',2);
 `); err != nil {
		t.Fatal(err)
	}
	return db, event.NewService(data.NewSQLiteEventStore(db))
}

func TestEventGoalAttributionWorkflow_SQLite(t *testing.T) {
	for _, path := range []string{"historical", "end", "successor"} {
		t.Run(path+" credits each matching Goal once", func(t *testing.T) {
			db, service := attributionFixture(t)
			var completed *data.Event
			var err error
			if path == "historical" {
				completed, err = service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
			} else {
				completed, err = service.Create(t.Context(), "owner", "Work", time.Unix(100, 0), []int64{1, 2})
				if err != nil {
					t.Fatal(err)
				}
				if path == "end" {
					completed, err = service.End(t.Context(), "owner", completed.EventID, completed.Version, time.Unix(200, 0))
				} else {
					_, err = service.Create(t.Context(), "owner", "Next", time.Unix(200, 0), nil)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			var recipients, supports int
			if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credits`).Scan(&recipients); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credit_subjects`).Scan(&supports); err != nil {
				t.Fatal(err)
			}
			if recipients != 2 || supports != 3 {
				t.Fatalf("recipients/supports: got %d/%d, want 2/3", recipients, supports)
			}
			var versions int64
			if err := db.QueryRow(`SELECT SUM(version) FROM goals WHERE user_id='owner'`).Scan(&versions); err != nil {
				t.Fatal(err)
			}
			if versions != 2 {
				t.Fatalf("Goal version sum: got %d, want unchanged 2", versions)
			}

		})
	}
}

func TestGoalProgressSummaryWorkflow_SQLite(t *testing.T) {
	t.Run("deduplicates diamonds, direct parent credit, equal Events, and independent manual rows", func(t *testing.T) {
		db, service := attributionFixture(t)
		if _, err := db.Exec(`
    INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (4,'owner','Parent',60),(5,'owner','Root',60);
    INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES (1,4,'owner',4);
    INSERT INTO goal_parents (goal_id,parent_goal_id,user_id) VALUES (1,4,'owner'),(2,4,'owner'),(1,5,'owner'),(4,5,'owner');
    INSERT INTO goal_progress (goal_id,user_id,time_spent_sec) VALUES (1,'owner',30),(1,'owner',30),(2,'owner',40);
  `); err != nil {
			t.Fatal(err)
		}
		for _, start := range []int64{100, 200} {
			if _, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(start, 0), time.Unix(start+100, 0), []int64{1, 2}); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteGoalProgressStore(db)
		got, err := store.GetGoalProgressSummary(t.Context(), "owner", 4)
		want := data.GoalProgressSummary{DirectAutomaticSeconds: 200, EffectiveAutomaticSeconds: 200, EffectiveManualSeconds: 100}
		if err != nil || got != want {
			t.Fatalf("parent summary: got %+v,%v, want %+v", got, err, want)
		}
		if _, err := db.Exec(`UPDATE goals SET goal_is_active=0, deleted_at=updated_at WHERE goal_id IN (1,2)`); err != nil {
			t.Fatal(err)
		}
		got, err = store.GetGoalProgressSummary(t.Context(), "owner", 5)
		if err != nil || got.EffectiveAutomaticSeconds != 200 || got.EffectiveManualSeconds != 100 {
			t.Fatalf("retained diamond: got %+v,%v, want automatic 200 manual 100", got, err)
		}
	})
	t.Run("preserves history across version gaps and suppresses recipients with an empty revision", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		update := func(subjects []int64, end int64) {
			t.Helper()
			until := time.Unix(end, 0)
			item, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Changed", StartedAt: item.StartedAt, EndedAt: &until, SubjectIDs: subjects})
			if err != nil {
				t.Fatal(err)
			}
		}
		update([]int64{2, 1}, 200)
		var revisions int
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_credit_revisions`).Scan(&revisions); err != nil {
			t.Fatal(err)
		}
		if revisions != 1 {
			t.Fatalf("label-only revisions: got %d, want 1", revisions)
		}
		update([]int64{1, 2}, 160)
		update(nil, 160)
		for _, id := range []int64{1, 2} {
			got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", id)
			if err != nil || got.EffectiveAutomaticSeconds != 0 {
				t.Fatalf("latest empty snapshot: got %+v,%v, want zero", got, err)
			}
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credits WHERE event_version=1`).Scan(&revisions); err != nil {
			t.Fatal(err)
		}
		if revisions != 2 {
			t.Fatalf("original immutable recipients: got %d, want 2", revisions)
		}
	})
	t.Run("Subject deletion and duration edits retain earned support", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "owner", 1); err != nil {
			t.Fatal(err)
		}
		item, err = service.Get(t.Context(), "owner", item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		end := time.Unix(160, 0)
		_, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Corrected", StartedAt: item.StartedAt, EndedAt: &end, SubjectIDs: item.SubjectIDs})
		if err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err != nil || got.DirectAutomaticSeconds != 60 {
			t.Fatalf("retained credit: got %+v,%v, want 60", got, err)
		}
	})
}

func TestEventGoalRetentionWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      int64
	}{
		{"inactive recipients retain earned credit", `UPDATE goals SET goal_is_active=0 WHERE goal_id=1`, 100},
		{"unlinked recipients retain earned credit", `UPDATE subject_goals SET deleted_at=1 WHERE goal_id=1`, 100},
		{"soft deleted Event stops counting", `UPDATE events SET deleted_at=updated_at`, 0},
		{"hard deleted Subject retains detached support", `DELETE FROM subjects WHERE subject_id=1`, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, service := attributionFixture(t)
			if _, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
			if err != nil || got.DirectAutomaticSeconds != tc.want {
				t.Fatalf("automatic time: got %+v,%v, want %d", got, err, tc.want)
			}
		})
	}
	t.Run("zero completion freezes recipients and no eligible completion is not backfilled", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Zero", time.Unix(100, 0), time.Unix(100, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE goals SET goal_is_active=0 WHERE goal_id=1`); err != nil {
			t.Fatal(err)
		}
		end := time.Unix(150, 0)
		if _, err := service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Longer", StartedAt: item.StartedAt, EndedAt: &end, SubjectIDs: item.SubjectIDs}); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err != nil || got.DirectAutomaticSeconds != 50 {
			t.Fatalf("zero recipient retention: got %+v,%v, want 50", got, err)
		}
		item, err = service.CreatePast(t.Context(), "owner", "No recipients", time.Unix(200, 0), time.Unix(250, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE goals SET goal_is_active=1 WHERE goal_id=1`); err != nil {
			t.Fatal(err)
		}
		end = time.Unix(260, 0)
		if _, err := service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Longer", StartedAt: item.StartedAt, EndedAt: &end, SubjectIDs: item.SubjectIDs}); err != nil {
			t.Fatal(err)
		}
		got, err = data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err != nil || got.DirectAutomaticSeconds != 50 {
			t.Fatalf("empty snapshot retention: got %+v,%v, want 50", got, err)
		}
	})
	t.Run("shared support survives removing one Subject even after Goal deactivation", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE goals SET goal_is_active=0 WHERE goal_id=1`); err != nil {
			t.Fatal(err)
		}
		_, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Work", StartedAt: item.StartedAt, EndedAt: item.EndedAt, SubjectIDs: []int64{2}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err != nil || got.DirectAutomaticSeconds != 100 {
			t.Fatalf("remaining earned support: got %+v,%v, want 100", got, err)
		}
	})
	t.Run("legacy edits establish credit only through additions", func(t *testing.T) {
		db, service := attributionFixture(t)
		if _, err := db.Exec(`INSERT INTO events (event_id,user_id,started_at,ended_at) VALUES (1,'owner',100,200);
    INSERT INTO event_subjects (event_id,subject_id,user_id) VALUES (1,1,'owner')`); err != nil {
			t.Fatal(err)
		}
		_, err := service.Update(t.Context(), "owner", 1, 1, event.UpdateInput{Activity: "Classified", StartedAt: time.Unix(100, 0), EndedAt: timePointer(200), SubjectIDs: []int64{1, 2}})
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credit_subjects WHERE subject_id=1`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("legacy unchanged Subject supports: got %d, want zero", n)
		}
	})
}

func timePointer(seconds int64) *time.Time { at := time.Unix(seconds, 0); return &at }

func TestEventGoalRollbackWorkflow_SQLite(t *testing.T) {
	for _, path := range []string{"historical", "end", "successor", "correction"} {
		t.Run(path+" rolls back when attribution fails", func(t *testing.T) {
			db, service := attributionFixture(t)
			var item *data.Event
			var err error
			if path == "end" || path == "successor" {
				item, err = service.Create(t.Context(), "owner", "Original", time.Unix(100, 0), []int64{1})
			}
			if path == "correction" {
				item, err = service.CreatePast(t.Context(), "owner", "Original", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TRIGGER reject_credit BEFORE INSERT ON event_credit_revisions BEGIN SELECT RAISE(ABORT,'controlled attribution failure'); END`); err != nil {
				t.Fatal(err)
			}
			switch path {
			case "historical":
				_, err = service.CreatePast(t.Context(), "owner", "New", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
			case "end":
				_, err = service.End(t.Context(), "owner", item.EventID, item.Version, time.Unix(200, 0))
			case "successor":
				_, err = service.Create(t.Context(), "owner", "Next", time.Unix(200, 0), nil)
			case "correction":
				_, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Changed", StartedAt: item.StartedAt, EndedAt: timePointer(160), SubjectIDs: item.SubjectIDs})
			}
			if err == nil {
				t.Fatal("attribution failure: got nil, want rollback")
			}
			if item != nil {
				got, err := service.Get(t.Context(), "owner", item.EventID)
				if err != nil || got.Version != item.Version || (got.EndedAt == nil) != (item.EndedAt == nil) {
					t.Fatalf("rollback: got %+v,%v, want original %+v", got, err, item)
				}
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 1
			if path == "historical" {
				want = 0
			}
			if count != want {
				t.Fatalf("Event count: got %d, want %d", count, want)
			}
		})
	}
}

func TestEventGoalBoundariesWorkflow_SQLite(t *testing.T) {
	t.Run("rejects unavailable summary roots and canceled writes without partial results", func(t *testing.T) {
		db, service := attributionFixture(t)
		store := data.NewSQLiteGoalProgressStore(db)
		for _, id := range []int64{3, 999} {
			if _, err := store.GetGoalProgressSummary(t.Context(), "owner", id); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("summary %d: got %v, want not found", id, err)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := service.CreatePast(ctx, "owner", "Canceled", time.Unix(100, 0), time.Unix(200, 0), []int64{1}); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: got %v, want canceled", err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_credit_revisions`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("canceled revisions: got %d, want zero", count)
		}
	})
	t.Run("reports integer overflow without rounded or partial totals", func(t *testing.T) {
		db, _ := attributionFixture(t)
		if _, err := db.Exec(`INSERT INTO goal_progress (goal_id,user_id,time_spent_sec) VALUES (1,'owner',9223372036854775807),(1,'owner',1)`); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err == nil || got != (data.GoalProgressSummary{}) {
			t.Fatalf("overflow: got %+v,%v, want error without partial totals", got, err)
		}
	})
	t.Run("old detached support is not confused with a reused Subject identifier", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM subjects WHERE subject_id=1;
    INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'owner','New identity');
    INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES (1,1,'owner',1)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE event_goal_credit_subjects SET subject_id=1`); err == nil {
			t.Fatal("detached support reattached")
		}
		item, err = service.Get(t.Context(), "owner", item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		for _, ids := range [][]int64{{1}, nil} {
			item, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Corrected", StartedAt: item.StartedAt, EndedAt: item.EndedAt, SubjectIDs: ids})
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err != nil || got.DirectAutomaticSeconds != 100 {
			t.Fatalf("detached historical support: got %+v,%v, want 100", got, err)
		}
	})
	t.Run("credits three Subjects while excluding retired and inactive recipients", func(t *testing.T) {
		db, service := attributionFixture(t)
		if _, err := db.Exec(`
    INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (4,'owner','Third');
    INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (4,'owner','Third goal',60);
    INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES (4,1,'owner',1),(4,4,'owner',2);
    UPDATE subject_goals SET deleted_at=1 WHERE goal_id=2;
  `); err != nil {
			t.Fatal(err)
		}
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2, 4})
		if err != nil {
			t.Fatal(err)
		}
		var recipients, supports int
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credits WHERE event_id=?`, item.EventID).Scan(&recipients); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credit_subjects WHERE goal_id=1`).Scan(&supports); err != nil {
			t.Fatal(err)
		}
		if recipients != 2 || supports != 3 {
			t.Fatalf("three-Subject union: got %d recipients/%d supports, want 2/3", recipients, supports)
		}
	})
}

func TestEventGoalEligibilityWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		subjects  []int64
	}{
		{"Misc does not credit Unassigned Goals", "", nil},
		{"inactive Goals receive no new credit", `UPDATE goals SET goal_is_active=0 WHERE goal_id=1`, []int64{1}},
		{"deleted Goals receive no new credit", `UPDATE goals SET deleted_at=updated_at WHERE goal_id=1`, []int64{1}},
		{"retired Goal links receive no new credit", `UPDATE subject_goals SET deleted_at=1 WHERE goal_id=1`, []int64{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, service := attributionFixture(t)
			if tc.sql != "" {
				if _, err := db.Exec(tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), tc.subjects); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM event_goal_credits`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("ineligible recipients: got %d, want zero", count)
			}
		})
	}
	t.Run("reopens durable attribution and changes inheritance without parent writes", func(t *testing.T) {
		db, service := attributionFixture(t)
		if _, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (4,'owner','Parent',60)`); err != nil {
			t.Fatal(err)
		}
		var sequence int
		var name, path string
		if err := db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, "file:"+path)
		progress := data.NewSQLiteGoalProgressStore(reopened)
		for _, edge := range []struct {
			sql  string
			want int64
		}{
			{`INSERT INTO goal_parents (goal_id,parent_goal_id,user_id) VALUES (1,4,'owner'),(2,4,'owner')`, 100},
			{`DELETE FROM goal_parents WHERE goal_id=1`, 100},
			{`DELETE FROM goal_parents WHERE goal_id=2`, 0},
		} {
			if _, err := db.Exec(edge.sql); err != nil {
				t.Fatal(err)
			}
			got, err := progress.GetGoalProgressSummary(t.Context(), "owner", 4)
			if err != nil || got.EffectiveAutomaticSeconds != edge.want {
				t.Fatalf("dynamic inheritance: got %+v,%v, want %d", got, err, edge.want)
			}
		}
	})
}

func TestEventGoalTransactionBoundaries_SQLite(t *testing.T) {
	t.Run("writer contention cannot partially establish attribution", func(t *testing.T) {
		db, _ := attributionFixture(t)
		var sequence int
		var name, path string
		if err := db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
			t.Fatal(err)
		}
		other := openSQLite(t, "file:"+path)
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(t.Context(), `UPDATE goals SET goal_name=goal_name WHERE goal_id=1`); err != nil {
			t.Fatal(err)
		}
		service := event.NewService(data.NewSQLiteEventStore(other))
		if _, err := service.CreatePast(t.Context(), "owner", "Contended", time.Unix(100, 0), time.Unix(200, 0), []int64{1}); !errors.Is(err, data.ErrDatabaseBusy) {
			t.Fatalf("contention: got %v, want database busy", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM event_credit_revisions`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("contended revisions: got %d, want zero", count)
		}
	})
	t.Run("fall back uses elapsed UTC time instead of repeated wall-clock labels", func(t *testing.T) {
		db, service := attributionFixture(t)
		start := time.Date(2026, time.November, 1, 8, 30, 0, 0, time.UTC)
		if _, err := service.CreatePast(t.Context(), "owner", "Repeated hour", start, start.Add(time.Hour), []int64{1, 2}); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if err != nil || got.DirectAutomaticSeconds != 3600 {
			t.Fatalf("elapsed seconds: got %+v,%v, want 3600", got, err)
		}
	})
}
