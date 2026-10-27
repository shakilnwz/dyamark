package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"dyamark/app"

	gioapp "gioui.org/app"
)

func main() {
	start := time.Now()
	startRSS, _ := app.RSSKB()

	measure := flag.Bool("measure", false, "print start-up time and memory after the first frame")
	exitAfter := flag.Bool("exit-after", false, "with --measure, quit after the first frame")
	flag.Parse()

	filePath := flag.Arg(0)

	go func() {
		a, err := app.New(app.Options{
			FilePath:   filePath,
			Measure:    *measure,
			ExitAfter:  *exitAfter,
			Start:      start,
			StartRSSKB: startRSS,
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
