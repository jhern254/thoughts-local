package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	cli "github.com/urfave/cli/v3"
	_ "modernc.org/sqlite"
)

const defaultSQLiteDSN = "file:data/thoughts.db"

type subjectCreatorFactory func(context.Context, string) (SubjectCreator, io.Closer, error)

func newCLI(out io.Writer, newSubjects subjectCreatorFactory) *cli.Command {
	return &cli.Command{
		Name:   "thoughts",
		Usage:  "capture and organize thoughts",
		Writer: out,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "user-id",
				Usage:    "owner of the requested resources",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "db-dsn",
				Usage:   "SQLite data source name",
				Value:   defaultSQLiteDSN,
				Sources: cli.EnvVars("THOUGHTS_DB_DSN"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "subjects",
				Usage: "manage subjects",
				Commands: []*cli.Command{
					{
						Name:            "create",
						Usage:           "create a subject",
						ArgsUsage:       "<name>",
						SkipFlagParsing: true,
						Action: func(ctx context.Context, cmd *cli.Command) (runErr error) {
							if cmd.NArg() != 1 {
								return fmt.Errorf("expected exactly one subject name")
							}
							dsn := cmd.String("db-dsn")
							if dsn == "" {
								dsn = defaultSQLiteDSN
							}
							subjects, closer, err := newSubjects(ctx, dsn)
							if err != nil {
								return err
							}
							defer func() {
								if closeErr := closer.Close(); closeErr != nil {
									runErr = errors.Join(runErr, closeErr)
								}
							}()

							command := SubjectCreateCommand{
								subjects: subjects,
								userID:   cmd.String("user-id"),
								out:      cmd.Root().Writer,
							}
							return command.Run(ctx, cmd.Args().Get(0))
						},
					},
				},
			},
		},
	}
}

func openSubjectCreator(ctx context.Context, dsn string) (SubjectCreator, io.Closer, error) {
	db, err := sql.Open("sqlite", sqliteDSNWithForeignKeys(dsn))
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, nil, err
	}

	return subject.NewService(data.NewSQLiteSubjectStore(db)), db, nil
}

func sqliteDSNWithForeignKeys(dsn string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "_pragma=foreign_keys(1)"
}
