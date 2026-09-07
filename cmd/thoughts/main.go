package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jhern254/go-thoughts/internal/logging"
)

func main() {
	logger, err := logging.NewConsole(os.Stderr, "thoughts-cli", os.Getenv("THOUGHTS_LOG_LEVEL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	app := newApplication(os.Stdout, os.Stderr, logger)
	if err := newCLI(app).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(app.errOut, err)
		os.Exit(1)
	}
}
