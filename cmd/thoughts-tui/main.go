package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	app := newApplication(os.Stdin, os.Stdout, os.Stderr)
	if err := newTUI(app).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(app.errOut, err)
		os.Exit(1)
	}
}
