package vlcServer

import (
	"fmt"
	"net/http"
)

func healthCheck(w http.ResponseWriter) {
	fmt.Fprintf(w, "OK")
	//http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
}

// https://stackoverflow.com/questions/15204162/check-if-a-process-exists-in-go-way
// func isPidRunning(int pid) bool {

// 	process, err := os.FindProcess(pid)
// 	if err != nil {
// 		fmt.Printf("Failed to find process: %s\n", err)
// 		return false
// 	} else {
// 		err := process.Signal(syscall.Signal(0))
// 		fmt.Printf("process.Signal on pid %d returned: %v\n", pid, err)
// 		return true
// 	}

// }
