#!/usr/bin/env bash

source .env
# CHANNEL_NAME=adanalife_

# curl -s -H "Client-ID: $TWITCH_CLIENT_ID" \
#   -H 'Content-Type: application/json' \
#   -H "Authorization: Bearer $USER_ACCESS_TOKEN" \
#   -X PUT "https://api.twitch.tv/helix/streams/tags?broadcaster_id=$CHANNEL_ID" \
#   -d '{"tag_ids": ["1400ca9c-84ea-414e-a85b-076a70d38ecf","77223888-8535-4614-974b-b1b2673456eb","a4fac2cc-7cd4-44a6-b620-178182389a5b","a6ff589a-33e5-4caf-8286-29dea98fc2e2" ,"89e105c9-2c45-42a9-a5f0-fc1ea6e7ba8b"]}'

CHANNEL_ID=$(curl -s http://localhost:8080/auth/twitch | jq -r .channel_id)

curl -s \
  -H "Client-ID: $TWITCH_CLIENT_ID" \
  -H 'Content-Type: application/json' \
  -X POST https://api.twitch.tv/helix/webhooks/hub \
  -d "{\"hub.callback\": \"http://tripbot.dana.lol/webhooks\", \"hub.mode\": \"subscribe\", \"hub.topic\": \"https://api.twitch.tv/helix/users/follows?first=1&to_id=$CHANNEL_ID\"}"
