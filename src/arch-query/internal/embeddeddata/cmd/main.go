package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jctanner/arch-query/internal/embeddeddata"
)

func main() {
	source := flag.String("source", "", "architecture source directory")
	destination := flag.String("destination", "", "staging destination directory")
	overlays := flag.String("overlays", "", "optional overlays source directory")
	flag.Parse()
	if *source == "" || *destination == "" {
		fmt.Fprintln(os.Stderr, "source and destination are required")
		os.Exit(2)
	}
	if err := embeddeddata.Stage(*source, *destination, *overlays); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
