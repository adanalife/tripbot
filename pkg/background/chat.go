package background

import (
	"fmt"
	"log"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var chatDuration = time.Duration(20 * time.Second)
var chatFile = path.Join(helpers.ProjectRoot(), "OBS/chat.txt")

var Chat *onscreens.Onscreen
var ChatLines = []string{}

func InitChat() {
	log.Println("Creating chat onscreen")
	Chat = onscreens.New(chatFile)
	// go chatLoop()
}

//TODO: actually use time
func AddChatLine(username, line string, t time.Time) {
	ChatLines = append(ChatLines, fmt.Sprintf("[%s] %s", username, line))
	Chat.Show(chatContent(), chatDuration)
}

// func chatLoop() {
// 	for { // forever
// 		if rand.Intn(10) == 0 {
// 			ShowChat()
// 		}
// 		time.Sleep(time.Duration(30 * time.Second))
// 	}
// }

// chatContent creates the content for the chat
func chatContent() string {

	var output string

	size := 5
	if len(ChatLines) < size {
		size = len(ChatLines)
	}
	lines := ChatLines[size:]

	// spew.Dump(ChatLines)
	// spew.Dump(lines)

	for _, line := range lines {
		output = output + "\n" + line
	}

	return output
}
