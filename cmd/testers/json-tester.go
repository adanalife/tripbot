package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
)

type viewers struct {
	Count    int `json:"chatter_count"`
	Chatters map[string][]string
}

func main() {
	twitch.UpdateViewers()
	fmt.Println("viewer count:", twitch.ViewerCount())
	fmt.Println("chatters:")
	spew.Dump(twitch.Chatters())
	// text := `{"_links":{},"chatter_count":17,"chatters":{"broadcaster":["adanalife_"],"vips":[],"moderators":["nightbot","streamlabs","tripbot4000"],"staff":[],"admins":[],"global_mods":[],"viewers":["23hagbard23","anotherttvviewer","chu0805","griphiny","joulut","kroxeldefik","logviewer","lurxx","razer_ridd","the_ambassador9","v_and_k","virgoproz","winsock"]}}`
	// textBytes := []byte(text)

	// v := viewers{}
	// err := json.Unmarshal(textBytes, &v)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// fmt.Println(v.Count)
	// spew.Dump(v.Chatters)
}

func printStruct(prefix string, m map[string]interface{}) {
	for k, v := range m {
		switch vv := v.(type) {
		case string:
			fmt.Println(prefix, k, "is string", vv)
		case float64:
			fmt.Println(prefix, k, "is number", vv)
		case []interface{}:
			fmt.Println(prefix, k, "is an array:")
			for i, u := range vv {
				fmt.Println(prefix, i, u)
			}
		case map[string]interface{}:
			fmt.Println(prefix, k, "is a map:")
			printStruct(prefix+"  ", vv)
		default:
			fmt.Println(prefix, k, "is a type we can't handle", vv)
		}
	}
}
