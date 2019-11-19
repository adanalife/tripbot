package twitch

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// chattersAPIURL is the URL to hit for current chatter list
// var chattersAPIURL = "https://tmi.twitch.tv/group/user/" + config.ChannelName + "/chatters"
var chattersAPIURL = "https://tmi.twitch.tv/group/user/adanalife_/chatters"

// chattersResponse is the json returned by the Twitch chatters endpoint
type chattersResponse struct {
	Count    int `json:"chatter_count"`
	Chatters map[string][]string
}

// currentChatters will contain the current viewers
var currentChatters chattersResponse

//TODO: is this even necessary?
func ViewerCount() int {
	return currentChatters.Count
}

// Chatters returns a slice containing all current chatters
func Chatters() []string {
	var chatters []string
	for _, list := range currentChatters.Chatters {
		for _, chatter := range list {
			chatters = append(chatters, chatter)
		}
	}
	return chatters
}

// UpdateViewers makes a request to the chatters API and updates currentChatters
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

	err = json.Unmarshal(body, &currentChatters)
	if err != nil {
		fmt.Println("error unmarshalling json", err)
	}
}
