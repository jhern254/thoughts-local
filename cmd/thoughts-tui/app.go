package main

import (
	"context"
	"io"

	tea "charm.land/bubbletea/v2"
	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/tui"
	cli "github.com/urfave/cli/v3"
)

const defaultSQLiteDSN = appcore.DefaultSQLiteDSN

type runtime interface {
	LocalUser() *data.User
	Subjects() *subject.Service
	Close() error
}

type application struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer

	runtime     runtime
	openRuntime func(context.Context, string) (runtime, error)
	runProgram  func(context.Context, tea.Model, io.Reader, io.Writer) error
}

func newApplication(in io.Reader, out, errOut io.Writer) *application {
	return &application{
		in:     in,
		out:    out,
		errOut: errOut,
		openRuntime: func(ctx context.Context, dsn string) (runtime, error) {
			return appcore.Open(ctx, dsn)
		},
		runProgram: runBubbleTea,
	}
}

func newTUI(app *application) *cli.Command {
	return &cli.Command{
		Name:      "thoughts-tui",
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
			dsn := cmd.String("db-dsn")
			if dsn == "" {
				dsn = defaultSQLiteDSN
			}
			runtime, err := app.openRuntime(ctx, dsn)
			if err != nil {
				return ctx, err
			}
			app.runtime = runtime
			return ctx, nil
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			return app.runProgram(
				ctx,
				tui.NewModel(ctx, app.runtime.LocalUser(), app.runtime.Subjects()),
				app.in,
				app.out,
			)
		},
		After: func(context.Context, *cli.Command) error {
			if app.runtime == nil {
				return nil
			}
			err := app.runtime.Close()
			app.runtime = nil
			return err
		},
	}
}

func runBubbleTea(ctx context.Context, model tea.Model, in io.Reader, out io.Writer) error {
	_, err := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(out),
	).Run()
	return err
}
