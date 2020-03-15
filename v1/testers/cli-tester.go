package main

import (
	"flag"
	"os"

	"github.com/davecgh/go-spew/spew"
)

func main() {
	// CommandLine.Parse()
	locationFlags := flag.NewFlagSet("location", flag.ContinueOnError)
	zoom := locationFlags.Bool("zoom", false, "")
	locationFlags.Parse(os.Args[1:])
	spew.Dump(locationFlags)
	spew.Dump(os.Args[1:])
	spew.Dump(silent)
}
