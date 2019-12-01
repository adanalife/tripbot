package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func countOpenFiles() int64 {
}

func main() {
	out, err := exec.Command("/bin/sh", "-c", fmt.Sprintf("lsof -p %v", os.Getpid())).Output()
	if err != nil {
		fmt.Println(err.Error())
	}
	lines := strings.Split(string(out), "\n")
	fmt.Println(getOpenfiles())
}
