package main

import (
	"os"

	"github.com/jonasbg/paste/pastectl/internal/cli"
)

func main() {
	app := cli.New()
	if err := app.Run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
