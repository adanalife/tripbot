package main

import (
	"log/slog"
	"syscall"

	"github.com/davecgh/go-spew/spew"
	"github.com/mitchellh/go-ps"
	"github.com/skratchdot/open-golang/open"
)

func main() {
	stopiTunes()
	startiTunes()
}

func stopiTunes() {
	itunesBinary := "iTunes"

	processes, err := ps.Processes()
	if err != nil {
		slog.Error("error getting pids", "err", err)
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
		slog.Info("killing iTunes", "pid", itunesProcess.Pid())
		err = syscall.Kill(itunesProcess.Pid(), syscall.SIGKILL)
		if err != nil {
			slog.Error("error killing pid", "err", err)
		}
	}
}

func startiTunes() {
	slog.Info("opening iTunes")
	open.RunWith("http://somafm.com/groovesalad256.pls", "iTunes")
}
