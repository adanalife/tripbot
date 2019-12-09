package chatbot

// handles all chat messages
func PrivateMessage(message twitch.PrivateMessage) {
	username := message.User.Name

	// log in the user
	user := users.LoginIfNecessary(username)

	if strings.HasPrefix(strings.ToLower(message.Message), "!help") {
		helpCmd(user)
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!uptime") {
		uptimeCmd(user)
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!oldmiles") {
		if user.HasCommandAvailable() {
			oldMilesCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	// any of these should trigger the miles command
	milesStrings := []string{
		"!miles",
		"!newmiles",
	}
	for _, s := range milesStrings {
		if strings.HasPrefix(strings.ToLower(message.Message), s) {
			if user.HasCommandAvailable() {
				milesCmd(user)
			} else {
				client.Say(config.ChannelName, followerMsg)
			}
		}
	}

	// any of these should trigger the kilometres command
	kilometresStrings := []string{
		"!km",
		"!kilometres",
		"!kilometers",
	}
	for _, s := range kilometresStrings {
		if strings.HasPrefix(strings.ToLower(message.Message), s) {
			if user.HasCommandAvailable() {
				kilometresCmd(user)
			} else {
				client.Say(config.ChannelName, followerMsg)
