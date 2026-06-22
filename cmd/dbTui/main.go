package main

import (
	"fmt"
	"os"

	"github.com/farhank15/dbTui/internal/tui"
)

func main() {
	app := tui.NewApp()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
