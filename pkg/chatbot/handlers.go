package chatbot

func runCommand(user users.User, message string) {
	split := strings.Split(message, " ")
	command := split[0]
	params := split[1:]
	switch command {
	case "!help":
		helpCmd(user)
	case "!uptime":
		uptimeCmd(user)
	case "!uptime":
		uptimeCmd(user)
	case "!oldmiles":
		if user.HasCommandAvailable() {
			oldMilesCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	default:
		err = fmt.Errorf("command %s not found", command)
	}
	if err != nil {
		terrors.Log(err, "error running command")
	}
}

// handles all chat messages
func PrivateMessage(message twitch.PrivateMessage) {
	username := message.User.Name
	//TODO: we lose capitalization here, is that okay?
	message := strings.ToLower(message.Message)

	// check to see if the message is a command
	//TODO: also include ones prefixed with whitespace?
	if strings.HasPrefix(message, "!") {
		// log in the user
		user := users.LoginIfNecessary(username)

		runCommand(user, message)
	}
}
