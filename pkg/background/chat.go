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

	size := 5
	// check to see if we even have enough lines
	if len(ChatLines) < size {
		size = len(ChatLines)
	}
	// get the last lines
	lines := ChatLines[len(ChatLines)-size:]

	// add all the lines together
	for _, line := range lines {
		output = output + "\n" + breakUpLine(line)
	}

	return output
}

// breakUpLine takes one long line and breaks it up
// into two lines of fixed width
func breakUpLine(fullLine string) string {
	var endpoint int
	line := ""
	lineLength := len(fullLine)

	// exit early if we don't need to break up the line
	if lineLength <= lineBreak {
		return fullLine
	}

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
	return line
}
