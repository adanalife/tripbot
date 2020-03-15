package main

import (
	"log"

	. "github.com/logrusorgru/aurora"
)

func main() {
	log.Printf("Got it %d times\n", Green(1240))
	log.Printf("PI is %+1.2e\n", Cyan(3.14))
}
