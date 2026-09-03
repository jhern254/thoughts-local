package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jhern254/go-thoughts/internal/data"
	cli "github.com/urfave/cli/v3"
)

type SubjectService interface {
	Create(ctx context.Context, userID, name string) (*data.Subject, error)
	Get(ctx context.Context, userID string, subjectID int64) (*data.Subject, error)
	List(ctx context.Context, userID string) ([]data.Subject, error)
	Update(ctx context.Context, userID string, subjectID int64, name string) (*data.Subject, error)
	Delete(ctx context.Context, userID string, subjectID int64) error
}

func newSubjectsCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:  "subjects",
		Usage: "manage subjects",
		Commands: []*cli.Command{
			newSubjectCreateCommand(app),
			newSubjectGetCommand(app),
			newSubjectListCommand(app),
			newSubjectUpdateCommand(app),
			newSubjectDeleteCommand(app),
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

func newSubjectGetCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "get a subject",
		ArgsUsage: "<id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 1 {
				return fmt.Errorf("expected exactly one subject ID")
			}
			subjectID, err := parseSubjectID(cmd.Args().Get(0))
			if err != nil {
				return err
			}

			found, err := app.subjects.Get(ctx, app.userID, subjectID)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(app.out, "Subject %d: %s\n", found.SubjectID, found.SubjectName)
			return err
		},
	}
}

func newSubjectListCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list subjects",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 0 {
				return fmt.Errorf("expected no arguments")
			}

			subjects, err := app.subjects.List(ctx, app.userID)
			if err != nil {
				return err
			}
			for _, item := range subjects {
				if _, err := fmt.Fprintf(app.out, "%d\t%s\n", item.SubjectID, item.SubjectName); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newSubjectUpdateCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:            "update",
		Usage:           "update a subject",
		ArgsUsage:       "<id> <name>",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 2 {
				return fmt.Errorf("expected subject ID and name")
			}
			subjectID, err := parseSubjectID(cmd.Args().Get(0))
			if err != nil {
				return err
			}

			updated, err := app.subjects.Update(ctx, app.userID, subjectID, cmd.Args().Get(1))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(app.out, "Updated subject %d: %s\n", updated.SubjectID, updated.SubjectName)
			return err
		},
	}
}

func newSubjectDeleteCommand(app *application) *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "delete a subject",
		ArgsUsage: "<id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 1 {
				return fmt.Errorf("expected exactly one subject ID")
			}
			subjectID, err := parseSubjectID(cmd.Args().Get(0))
			if err != nil {
				return err
			}

			if err := app.subjects.Delete(ctx, app.userID, subjectID); err != nil {
				return err
			}
			_, err = fmt.Fprintf(app.out, "Deleted subject %d\n", subjectID)
			return err
		},
	}
}

func parseSubjectID(value string) (int64, error) {
	subjectID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || subjectID <= 0 {
		return 0, fmt.Errorf("invalid subject ID %q: must be a positive integer", value)
	}
	return subjectID, nil
}
