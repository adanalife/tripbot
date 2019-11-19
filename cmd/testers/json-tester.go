package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
)

func main() {
	twitch.UpdateViewers()
	fmt.Println("viewer count:", twitch.ViewerCount())
	fmt.Println("chatters:")
	spew.Dump(twitch.Chatters())
}
