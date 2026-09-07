package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	logger, logFile, err := newLogger(defaultTUILogPath, os.Getenv("THOUGHTS_LOG_LEVEL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	app := newApplication(os.Stdin, os.Stdout, os.Stderr, logger)
	exitCode := 0
	if err := newTUI(app).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(app.errOut, err)
		exitCode = 1
	}
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			fmt.Fprintln(app.errOut, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
