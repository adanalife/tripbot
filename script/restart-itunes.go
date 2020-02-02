package main

import (
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/mitchellh/go-ps"
)

func main() {

	// restartCmd := "open -a iTunes http://somafm.com/groovesalad256.pls"
	// spew.Dump(restartCmd)

	itunesBinary := "/Applications/iTunes.app/Contents/MacOS/iTunes"
	// spew.Dump(ps.Processes())

	processes, err := ps.Processes()
	if err != nil {
		log.Println("error getting pids", err)
	}

	for _, p := range processes {
		if p.Executable() == itunesBinary {
			spew.Dump(p)
		}
	}

}
