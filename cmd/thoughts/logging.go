package main

import (
	"io"

	"github.com/jhern254/go-thoughts/internal/logging"
)

const cliApplicationName = "thoughts-cli"

func newLogger(output io.Writer, value string) (logging.Logger, error) {
	return logging.NewConsole(output, cliApplicationName, value)
}
