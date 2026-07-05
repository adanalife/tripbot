package twitch

// ChannelID returns the cached twitch-internal user ID for the channel, seeded
// by SetChannelID; "" until then. Exposed for cmd/tripbot's EventSub
// subscription setup.
func (cl *API) ChannelID() string {
	return cl.channelID
}

// SetChannelID seeds the cached channel ID from out-of-band (the
// platform-gateway's /v1/users/{login}). The gateway owns Helix, so nothing
// resolves the ID in-process any more; EventSub setup would otherwise see "".
func (cl *API) SetChannelID(id string) {
	cl.channelID = id
}

// ChatterCount returns the number of chatters as reported by Twitch, cached
// from the gateway via SetChatters.
func (cl *API) ChatterCount() int {
	return cl.chatterCount
}

// Chatters returns a set of current chatter logins, cached from the gateway via
// SetChatters.
func (cl *API) Chatters() map[string]struct{} {
	chatters := make(map[string]struct{})
	for _, chatter := range cl.currentChatters {
		chatters[chatter.UserLogin] = struct{}{}
	}
	return chatters
}
