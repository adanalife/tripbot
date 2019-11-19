package twitch

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
)

// chattersAPIURL is the URL to hit for current chatter list
var chattersAPIURL = "https://tmi.twitch.tv/group/user/" + config.ChannelName + "/chatters"

// chattersResponse is the json returned by the Twitch chatters endpoint
type chattersResponse struct {
	Count    int `json:"chatter_count"`
	Chatters map[string][]string
}

// currentChatters will contain the current viewers
var currentChatters chattersResponse

//TODO: is this even necessary?
func ChatterCount() int {
	return currentChatters.Count
}

// Chatters returns a map where the keys are current chatters
// we use an empty struct for performance reasons
// c.p. https://stackoverflow.com/a/10486196
//TODO: consider using an int as the value and have that be the ID in the DB
func Chatters() map[string]struct{} {
	//TODO: maybe we don't want to make this every time?
	var chatters = make(map[string]struct{})
	for _, list := range currentChatters.Chatters {
		for _, chatter := range list {
			chatters[chatter] = struct{}{}
		}
	}
	return chatters
}

// UpdateChatters makes a request to the chatters API and updates currentChatters
func UpdateChatters() {
	var latestChatters chattersResponse

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

	err = json.Unmarshal(body, &latestChatters)
	if err != nil {
		fmt.Println("error unmarshalling json", err)
	}

	currentChatters = latestChatters
}
