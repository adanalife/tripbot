#!/usr/bin/env bash

RESP=$(curl -s 'http://localhost:8080/auth/twitch?auth=yes')
echo $RESP

CHANNEL_ID=$(echo $RESP | jq -r .channel_id)
CLIENT_ID=$(echo $RESP | jq -r .client_id)

curl -s \
  -H "Client-ID: $CLIENT_ID" \
  -H 'Content-Type: application/json' \
  -X POST https://api.twitch.tv/helix/webhooks/hub \
  -d "{\"hub.callback\": \"http://tripbot.dana.lol/webhooks\", \"hub.mode\": \"subscribe\", \"hub.topic\": \"https://api.twitch.tv/helix/users/follows?first=1&to_id=$CHANNEL_ID\"}"
