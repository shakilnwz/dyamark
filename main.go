package main

import (
	"flag"
	"fmt"
	"os"

	"dyamark/app"
	gioapp "gioui.org/app"
)

func main() {
	measure := flag.Bool("measure", false, "measure startup time and memory RSS")
	exitAfter := flag.Bool("exit-after", false, "exit immediately after measuring first paint")
	flag.Parse()

	go func() {
		a, err := app.New(app.Options{
			Measure:   *measure,
			ExitAfter: *exitAfter,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error initializing dyamark: %v\n", err)
			os.Exit(1)
		}
		if err := a.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error running dyamark: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()

	gioapp.Main()
}
