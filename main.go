package main

import (
	"fmt"
	"os"

	"github.com/rancher/ci-image/internal/cli"
	"github.com/rancher/ci-image/internal/clierr"
)

func main() {
	if err := cli.Execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, clierr.Render("error", err))
		os.Exit(1)
	}
}
