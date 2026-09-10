package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jhern254/go-thoughts/internal/logging"
)

func main() {
	logger, logFile, err := logging.NewFile("data/thoughts-tui.log", "thoughts-tui", os.Getenv("THOUGHTS_LOG_LEVEL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not initialize application logging.")
		os.Exit(1)
	}

	app := newApplication(os.Stdin, os.Stdout, os.Stderr, logger)
	exitCode := 0
	if err := newTUI(app).Run(context.Background(), os.Args); err != nil {
		exitCode = app.reportError(err)
	}
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			fmt.Fprintln(app.errOut, "Could not close the application log file.")
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
