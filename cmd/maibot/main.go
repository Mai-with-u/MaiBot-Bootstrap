package main

import (
	"fmt"
	"os"

	"maibot/internal/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
		os.Exit(1)
	}
	a.Run(os.Args[1:])
}
