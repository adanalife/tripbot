package main

import (
	"log"
	"syscall"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/mitchellh/go-ps"
	"github.com/skratchdot/open-golang/open"
)

func main() {

	itunesBinary := "iTunes"

	processes, err := ps.Processes()
	if err != nil {
		log.Println("error getting pids", err)
	}

	//spew.Dump(processes)

	var itunesProcess ps.Process
	for _, p := range processes {
		if p.Executable() == itunesBinary {
			itunesProcess = p
			spew.Dump(p)
			// there probably isn't a second iTunes process
			break
		}
	}

	if itunesProcess != nil {
		fmt.Println("pid for iTunes is:", itunesProcess.Pid())
		fmt.Println("killing it...")
		err = syscall.Kill(itunesProcess.Pid(), syscall.SIGKILL)
		if err != nil {
			log.Println("error killing pid", err)
		}
	}


	// restartCmd := "open -a iTunes http://somafm.com/groovesalad256.pls"
	// spew.Dump(restartCmd)

	fmt.Println("opening iTunes")
	open.RunWith("http://somafm.com/groovesalad256.pls", "iTunes")
}
