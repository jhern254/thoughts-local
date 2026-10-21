package migrations

import (
	"strings"
	"testing"
)

func TestMigrations_SubjectAliases(t *testing.T) {
	t.Run("enforces exact names across aliases and canonical subjects", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		for _, statement := range []string{
			`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES(1,'user-1','Go Programming'),(2,'user-1','Writing'),(3,'user-2','Go Programming')`,
			`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'user-1','Golang'),(1,'user-1','coding'),(1,'user-1','coDing'),(3,'user-2','Golang')`,
		} {
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
		for _, statement := range []string{
			`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'user-1','Go Programming')`,
			`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'user-1','Writing')`,
			`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(2,'user-1','Golang')`,
			`UPDATE subject_aliases SET alias_name='Writing' WHERE user_id='user-1' AND alias_name='Golang'`,
			`UPDATE subject_aliases SET alias_name='coding' WHERE user_id='user-1' AND alias_name='Golang'`,
			`INSERT INTO subjects(user_id,subject_name) VALUES('user-1','Golang')`,
			`UPDATE subjects SET subject_name='Golang' WHERE subject_id=2`,
			`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'user-2','foreign')`,
		} {
			if _, err := db.Exec(statement); err == nil {
				t.Fatalf("write %q: got nil, want constraint failure", statement)
			}
		}
		if _, err := db.Exec(`UPDATE subject_aliases SET alias_name='Golang' WHERE user_id='user-1' AND alias_name='Golang'`); err != nil {
			t.Fatalf("unchanged alias: got %v, want success", err)
		}
	})

	t.Run("matches subject name and timestamp constraints", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES(1,'user-1','Primary')`); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"", "   ", strings.Repeat("界", 256)} {
			if _, err := db.Exec(`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'user-1',?)`, name); err == nil {
				t.Fatalf("name length %d: got nil, want constraint failure", len(name))
			}
		}
		if _, err := db.Exec(`INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'user-1',?)`, "  "+strings.Repeat("界", 255)+"  "); err != nil {
			t.Fatal(err)
		}
		for _, assignment := range []string{"updated_at=created_at-1", "deleted_at=created_at-1", "deleted_at=updated_at+1", "deleted_at='invalid'"} {
			if _, err := db.Exec(`UPDATE subject_aliases SET ` + assignment); err == nil {
				t.Fatalf("timestamps %s: got nil, want constraint failure", assignment)
			}
		}
	})

	t.Run("restoring deleted names cannot bypass namespace guards", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES(1,'user-1','Primary');
 INSERT INTO subjects(subject_id,user_id,subject_name,deleted_at) VALUES(2,'user-1','Former primary',unixepoch('now'));
 INSERT INTO subject_aliases(alias_id,subject_id,user_id,alias_name) VALUES(1,1,'user-1','Former primary');
 INSERT INTO subject_aliases(alias_id,subject_id,user_id,alias_name,deleted_at) VALUES(2,1,'user-1','Reserved',unixepoch('now'));
 INSERT INTO subjects(subject_id,user_id,subject_name) VALUES(3,'user-1','Reserved')`); err != nil {
			t.Fatal(err)
		}
		for _, statement := range []string{
			`UPDATE subjects SET deleted_at=NULL WHERE subject_id=2`,
			`UPDATE subject_aliases SET deleted_at=NULL WHERE alias_id=2`,
		} {
			if _, err := db.Exec(statement); err == nil {
				t.Fatalf("restore %q: got nil, want namespace conflict", statement)
			}
		}
	})

	t.Run("rollback removes namespace guards and reapplication restores them", func(t *testing.T) {
		db := openMigratedDatabase(t)
		applyMigrationFiles(t, db, "000013*.down.sql", true)
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='subject_aliases'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("alias table count: got %d, want 0", count)
		}
		insertUsers(t, db)
		if _, err := db.Exec(`INSERT INTO subjects(user_id,subject_name) VALUES('user-1','After rollback')`); err != nil {
			t.Fatalf("canonical write after rollback: %v", err)
		}
		applyMigrationFiles(t, db, "000013*.up.sql", false)
		if _, err := db.Exec(`INSERT INTO subject_aliases(subject_id,user_id,alias_name) SELECT subject_id,user_id,'Restored' FROM subjects`); err != nil {
			t.Fatalf("alias write after reapplication: %v", err)
		}
	})
}
