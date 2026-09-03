package main

import (
	"context"
	"fmt"
	"io"

	"github.com/jhern254/go-thoughts/internal/data"
)

type SubjectCreator interface {
	Create(ctx context.Context, userID, name string) (*data.Subject, error)
}

type SubjectCreateCommand struct {
	subjects SubjectCreator
	userID   string
	out      io.Writer
}

func (c SubjectCreateCommand) Run(ctx context.Context, name string) error {
	created, err := c.subjects.Create(ctx, c.userID, name)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.out, "Created subject %d: %s\n", created.SubjectID, created.SubjectName)
	return err
}
