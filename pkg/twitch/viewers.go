package twitch

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/davecgh/go-spew/spew"
)

//TODO: make this use ChannelName instead of hardcoding it
const chattersAPIURL = "https://tmi.twitch.tv/group/user/adanalife_/chatters"

// this is the json returned by the Twitch chatters endpoint
type chattersResponse struct {
	count    int `json:"chatter_count"`
	chatters map[string][]string
}

// this will contain the current viewers
var currentChatters chattersResponse

//TODO: is this even necessary?
func ViewerCount() int {
	return currentChatters.count
}

func Chatters() []string {
	var chatters []string
	for _, list := range currentChatters.chatters {
		for _, chatter := range list {
			chatters = append(chatters, chatter)
		}
	}
	return chatters
}

func UpdateViewers() {
	client := http.Client{
		Timeout: 2 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, chattersAPIURL, nil)
	if err != nil {
		log.Println("error creating request", err)
		return
	}

	// req.Header.Set("User-Agent", "tripbot")

	res, err := client.Do(req)
	if err != nil {
		log.Println("error making request", err)
		return
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println("error reading request body", err)
		return
	}

	spew.Dump(body)

	err = json.Unmarshal(body, &currentChatters)
	if err != nil {
		fmt.Println("error unmarshalling json", err)
	}
}
