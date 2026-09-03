package main

import (
	"context"
	"fmt"

	"github.com/jhern254/go-thoughts/internal/data"
	cli "github.com/urfave/cli/v3"
)

type SubjectService interface {
	Create(ctx context.Context, userID, name string) (*data.Subject, error)
}

func newSubjectsCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:  "subjects",
		Usage: "manage subjects",
		Commands: []*cli.Command{
			newSubjectCreateCommand(app),
		},
	}
}

func newSubjectCreateCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:            "create",
		Usage:           "create a subject",
		ArgsUsage:       "<name>",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 1 {
				return fmt.Errorf("expected exactly one subject name")
			}

			created, err := app.subjects.Create(ctx, app.userID, cmd.Args().Get(0))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(app.out, "Created subject %d: %s\n", created.SubjectID, created.SubjectName)
			return err
		},
	}
}
