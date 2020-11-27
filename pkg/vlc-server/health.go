package vlcServer

import (
	"fmt"
	"net/http"
	"os"
	"syscall"

	"github.com/adanalife/tripbot/pkg/helpers"
)

func healthCheck(w http.ResponseWriter) {
	obsPid := helpers.ReadPidFile(OBSPidFile)
	if !isPidRunning(obsPid) {
		http.Error(w, "OBS not running", http.StatusFailedDependency)
	}
	fmt.Fprintf(w, "OK")
}

// https://stackoverflow.com/questions/15204162/check-if-a-process-exists-in-go-way
func isPidRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("Failed to find process: %s\n", err)
		return false
	} else {
		err := process.Signal(syscall.Signal(0))
		fmt.Printf("process.Signal on pid %d returned: %v\n", pid, err)
		return true
	}
}
