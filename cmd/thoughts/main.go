package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	app := newApplication(os.Stdout, os.Stderr)
	if err := newCLI(app).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(app.errOut, err)
		os.Exit(1)
	}
}
