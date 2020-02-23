package background

import (
	"fmt"
	"log"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

const lineBreak = 40

var chatDuration = time.Duration(40 * time.Second)
var chatFile = path.Join(helpers.ProjectRoot(), "OBS/chat.txt")

var Chat *onscreens.Onscreen
var ChatLines = []string{}

func InitChat() {
	log.Println("Creating chat onscreen")
	Chat = onscreens.New(chatFile)
}

//TODO: actually use time
func AddChatLine(username, line string, t time.Time) {
	ChatLines = append(ChatLines, fmt.Sprintf("[%s] %s", username, line))
	Chat.Show(chatContent(), chatDuration)
}

// chatContent creates the content for the chat
func chatContent() string {
	var output string
	var endpoint int

	size := 5
	// check to see if we even have enough lines
	if len(ChatLines) < size {
		size = len(ChatLines)
	}
	// get the last lines
	lines := ChatLines[len(ChatLines)-size:]

	// spew.Dump(ChatLines)
	// spew.Dump(lines)

	// add all the lines together
	for _, fullLine := range lines {
		line := ""
		lineLength := len(fullLine)
		if lineLength > lineBreak {
			// include the first characters
			line = fullLine[:lineBreak]
			// add a newline and an indent
			line += "\n  "
			// we want to add one more line (subracting 2 for the indent)
			endpoint = lineBreak + lineBreak - 2
			// but sometimes the endpoint is beyond the size of the line
			if endpoint > lineLength {
				// in which case we should just use the end of the line
				endpoint = lineLength
			}
			line += fullLine[lineBreak:endpoint]
			//TODO: consider adding a "..." after really long messages
		}
		output = output + "\n" + line
	}

	return output
}
