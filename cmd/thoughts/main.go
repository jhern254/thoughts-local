package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := newCLI(os.Stdout, openSubjectCreator).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
