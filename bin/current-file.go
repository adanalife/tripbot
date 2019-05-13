package main

import (
	"log"
	"fmt"
	"os/exec"
)

func main() {
	pathToShellScript := "/Users/dmerrick/other_projects/danalol-stream/bin/current-file.sh"
	out, err := exec.Command(pathToShellScript).Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(out))
}
