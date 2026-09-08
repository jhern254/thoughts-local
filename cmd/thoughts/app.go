package main

import (
	"context"

	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	cli "github.com/urfave/cli/v3"
)

const defaultSQLiteDSN = appcore.DefaultSQLiteDSN

func newCLI(app *application) *cli.Command {
	return &cli.Command{
		Name:      "thoughts",
		Usage:     "capture and organize thoughts",
		Writer:    app.out,
		ErrWriter: app.errOut,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "db-dsn",
				Usage:   "SQLite data source name",
				Value:   defaultSQLiteDSN,
				Sources: cli.EnvVars("THOUGHTS_DB_DSN"),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			app.logger.Started()
			dsn := cmd.String("db-dsn")
			if dsn == "" {
				dsn = defaultSQLiteDSN
			}
			if err := app.open(ctx, dsn); err != nil {
				if category, emit := failure.Classify(logging.ApplicationStart, err); emit {
					app.logger.Failure(logging.ApplicationStart, category)
				}
				return ctx, err
			}
			return ctx, nil
		},
		After: func(context.Context, *cli.Command) error {
			err := app.close()
			if err != nil {
				if category, emit := failure.Classify(logging.ApplicationClose, err); emit {
					app.logger.Failure(logging.ApplicationClose, category)
				}
			}
			app.logger.Stopped()
			return err
		},
		Commands: []*cli.Command{
			newSubjectsCommand(app),
		},
	}
}
