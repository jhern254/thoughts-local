package main

import (
	"context"

	cli "github.com/urfave/cli/v3"
)

const defaultSQLiteDSN = "file:data/thoughts.db"

func newCLI(app *application) *cli.Command {
	return &cli.Command{
		Name:      "thoughts",
		Usage:     "capture and organize thoughts",
		Writer:    app.out,
		ErrWriter: app.errOut,
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
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			dsn := cmd.String("db-dsn")
			if dsn == "" {
				dsn = defaultSQLiteDSN
			}
			return ctx, app.open(ctx, dsn, cmd.String("user-id"))
		},
		After: func(context.Context, *cli.Command) error {
			return app.close()
		},
		Commands: []*cli.Command{
			newSubjectsCommand(app),
		},
	}
}
