#!/usr/bin/env bash

SERVER_URL="http://tripbot.dana.lol:3458"
LEASE_SECONDS=300

CALLBACK_URL="$SERVER_URL/webhooks/twitch/users/follows"

RESP=$(curl -s "$SERVER_URL/auth/twitch?auth=yes")
# RESP=$(curl -s 'http://localhost:8080/auth/twitch?auth=yes')
echo $RESP

CHANNEL_ID=$(echo $RESP | jq -r .channel_id)
CLIENT_ID=$(echo $RESP | jq -r .client_id)
APP_ACCESS_TOKEN=$(echo $RESP | jq -r .app_access_token)

curl -s \
  -H "Client-ID: $CLIENT_ID" \
  -H "Authorization: Bearer $APP_ACCESS_TOKEN" \
  -X GET 'https://api.twitch.tv/helix/webhooks/subscriptions' \
  | jq

curl -s \
  -H "Client-ID: $CLIENT_ID" \
  -H 'Content-Type: application/json' \
  -X POST https://api.twitch.tv/helix/webhooks/hub \
  -d "{\"hub.callback\": \"$CALLBACK_URL\", \"hub.mode\": \"subscribe\", \"hub.topic\": \"https://api.twitch.tv/helix/users/follows?first=1&to_id=$CHANNEL_ID\", \"hub.lease_seconds\": 300}"
  # -d "{\"hub.callback\": \"$CALLBACK_URL\", \"hub.mode\": \"subscribe\", \"hub.topic\": \"https://api.twitch.tv/helix/users/follows?first=1&to_id=$CHANNEL_ID\", \"hub.lease_seconds\": $LEASE_SECONDS}"

sleep 1

curl -s \
  -H "Client-ID: $CLIENT_ID" \
  -H "Authorization: Bearer $APP_ACCESS_TOKEN" \
  -X GET 'https://api.twitch.tv/helix/webhooks/subscriptions' \
  | jq
