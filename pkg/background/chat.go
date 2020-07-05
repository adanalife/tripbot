package background

import (
	"fmt"
	"log"
	"path"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/adanalife/tripbot/pkg/onscreens"
)

const lineBreak = 40
const maxLines = 5

var chatDuration = time.Duration(40 * time.Second)
var chatFile = path.Join(config.RunDir, "chat.txt")

var Chat *onscreens.Onscreen
var ChatLines = []string{}

func InitChat() {
	log.Println("Creating chat onscreen")
	Chat = onscreens.New(chatFile)
}

func AddChatLine(username, line string) {
	ChatLines = append(ChatLines, fmt.Sprintf("[%s] %s", username, line))
	Chat.ShowFor(chatContent(), chatDuration)
}

// chatContent creates the content for the chat
func chatContent() string {
	var output string

	// check to see if we even have enough lines
	size := maxLines
	if len(ChatLines) < size {
		size = len(ChatLines)
	}
	// get the last lines, resetting size
	ChatLines = ChatLines[len(ChatLines)-size:]

	// add all the lines together
	for _, line := range ChatLines {
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
