package main

import (
	"context"
	"errors"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/cmd/internal/cliutil"
	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/visual"
	cli "github.com/urfave/cli/v3"
)

const defaultSQLiteDSN = appcore.DefaultSQLiteDSN

type runtime interface {
	LocalUser() *data.User
	Subjects() *subject.Service
	Thoughts() *thought.Service
	Metrics() *metrics.Service
	Events() *event.Service
	TimelineView() *timeline.Service
	BrowserVisual(context.Context) (*visual.Service, error)
	Close() error
}

type application struct {
	in             io.Reader
	out            io.Writer
	errOut         io.Writer
	logger         logging.Logger
	failureMessage string

	runtime     runtime
	openRuntime func(context.Context, string) (runtime, error)
	runProgram  func(context.Context, tea.Model, io.Reader, io.Writer) error
	runBrowser  func(context.Context, int, bool, func(context.Context) tea.Model, io.Writer, logging.Logger, *visual.Service) error
}

func newApplication(in io.Reader, out, errOut io.Writer, logger logging.Logger) *application {
	return &application{
		in:     in,
		out:    out,
		errOut: errOut,
		logger: logger,
		openRuntime: func(ctx context.Context, dsn string) (runtime, error) {
			return appcore.Open(ctx, dsn)
		},
		runProgram: runBubbleTea,
		runBrowser: runBrowser,
	}
}

func newTUI(app *application) *cli.Command {
	cmd := &cli.Command{
		Name:   "thoughts-tui",
		Usage:  "capture and organize thoughts",
		Writer: app.out,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "browser", Usage: "serve the TUI in a local browser"},
			&cli.BoolFlag{Name: "browser-open", Usage: "open the default browser when serving", Value: true, Sources: cli.EnvVars("THOUGHTS_BROWSER_OPEN")},
			&cli.IntFlag{Name: "browser-port", Usage: "loopback browser port", Value: 7777, Sources: cli.EnvVars("THOUGHTS_BROWSER_PORT")},
			&cli.StringFlag{
				Name:        "db-dsn",
				Usage:       "SQLite data source name",
				Value:       defaultSQLiteDSN,
				DefaultText: "configured database",
				Sources:     cli.EnvVars("THOUGHTS_DB_DSN"),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if cmd.Bool("browser") && (cmd.Int("browser-port") < 1 || cmd.Int("browser-port") > 65535) {
				app.failureMessage = "Browser port must be between 1 and 65535."
				return ctx, errors.New("invalid browser port")
			}
			app.logger.Started()
			dsn := cmd.String("db-dsn")
			if dsn == "" {
				dsn = defaultSQLiteDSN
			}
			runtime, err := app.openRuntime(ctx, dsn)
			if err != nil {
				app.failureMessage = "Could not start the application."
				if category, emit := failure.Classify(logging.ApplicationStart, err); emit {
					app.logger.Failure(logging.ApplicationStart, category)
				}
				return ctx, err
			}
			app.runtime = runtime
			return ctx, nil
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			newModel := func(ctx context.Context) tea.Model {
				return tui.NewModel(ctx, app.runtime.LocalUser(), app.runtime.Subjects(), app.runtime.Thoughts(), app.runtime.Metrics(), app.runtime.Events(), app.runtime.TimelineView(), app.logger)
			}
			var err error
			if cmd.Bool("browser") {
				var backgrounds *visual.Service
				backgrounds, err = app.runtime.BrowserVisual(ctx)
				if err != nil {
					app.failureMessage = "Could not load browser visual."
					return err
				}
				err = app.runBrowser(ctx, cmd.Int("browser-port"), cmd.Bool("browser-open"), newModel, app.out, app.logger, backgrounds)
			} else {
				sessionCtx, cancel := context.WithCancel(ctx)
				session := tui.NewSession(sessionCtx, cancel, newModel)
				defer session.Stop()
				err = app.runProgram(sessionCtx, session, app.in, app.out)
				session.Stop()
				if session.Failed() {
					err = errors.New("terminal program failed")
				}
			}
			if err != nil {
				app.failureMessage = "Could not run the terminal interface."
				if cmd.Bool("browser") {
					app.failureMessage = "Could not run the browser interface."
				}
				if category, emit := failure.Classify(logging.TUIRun, err); emit {
					app.logger.Failure(logging.TUIRun, category)
				}
			}
			return err
		},
		After: func(context.Context, *cli.Command) error {
			if app.runtime == nil {
				app.logger.Stopped()
				return nil
			}
			err := app.runtime.Close()
			app.runtime = nil
			if err != nil {
				app.failureMessage = "Could not close the application database."
				if category, emit := failure.Classify(logging.ApplicationClose, err); emit {
					app.logger.Failure(logging.ApplicationClose, category)
				}
			}
			app.logger.Stopped()
			return err
		},
	}
	cliutil.ConfigureDiagnostics(cmd, &app.failureMessage)
	return cmd
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
