package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
	cli "github.com/urfave/cli/v3"
)

type MetricsService interface {
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCount, error)
}

func newMetricsCommand(app *application) *cli.Command {
	return &cli.Command{Name: "metrics", Usage: "query aggregate metrics", Commands: []*cli.Command{
		{Name: "thought-counts", Usage: "list subject IDs and thought counts (tab-separated)", Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 0 {
				app.failureMessage = "Expected no arguments."
				return errors.New("expected no arguments")
			}
			counts, err := app.metrics.ThoughtCountsBySubject(ctx, app.userID)
			if err != nil {
				app.failureMessage = "Could not load thought counts."
				if category, emit := failure.Classify(logging.ThoughtCountsBySubject, err); emit {
					app.logger.Failure(logging.ThoughtCountsBySubject, category)
				}
				return err
			}
			app.failureMessage = "Could not write command output."
			for _, count := range counts {
				if _, err := fmt.Fprintf(app.out, "%d\t%d\n", count.SubjectID, count.Count); err != nil {
					return err
				}
			}
			return nil
		}},
	}}
}
