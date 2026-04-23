package main

import (
	"log"
	"os"

	"github.com/rancher/ci-image/internal/cli"
)

func main() {
	if err := cli.Execute(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
