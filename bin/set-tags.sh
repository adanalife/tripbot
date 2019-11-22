#!/usr/bin/env bash

source .env
# CHANNEL_NAME=adanalife_

# get the broadcaster ID
CHANNEL_ID=$(curl -s -H "Client-ID: $TWITCH_CLIENT_ID" -X GET "https://api.twitch.tv/helix/users?login=$CHANNEL_NAME" | jq -r .data[0].id)
echo $CHANNEL_ID

# to get the current tags:
# TAGS=$(curl -s -H "Client-ID: $TWITCH_CLIENT_ID" -X GET "https://api.twitch.tv/helix/streams/tags?broadcaster_id=$CHANNEL_ID" | jq -c '[.data[].tag_id]')
# echo $TAGS


# curl -s "https://id.twitch.tv/oauth2/token?client_id=$TWITCH_CLIENT_ID&client_secret=$TWITCH_AUTH_TOKEN&grant_type=client_credentials"



# 6ea6bca4-4712-4ab9-a906-e3336a9d8039 english (not required)
# 1400ca9c-84ea-414e-a85b-076a70d38ecf automotive
# 77223888-8535-4614-974b-b1b2673456eb visual asmr
# a4fac2cc-7cd4-44a6-b620-178182389a5b exploration
# a6ff589a-33e5-4caf-8286-29dea98fc2e2 travel
# 89e105c9-2c45-42a9-a5f0-fc1ea6e7ba8b outdoors

curl -s -H "Client-ID: $TWITCH_CLIENT_ID" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TWITCH_AUTH_TOKEN" \
  -X PUT "https://api.twitch.tv/helix/streams/tags?broadcaster_id=$CHANNEL_ID" \
  -d '{"tag_ids": ["1400ca9c-84ea-414e-a85b-076a70d38ecf","77223888-8535-4614-974b-b1b2673456eb","a4fac2cc-7cd4-44a6-b620-178182389a5b","a6ff589a-33e5-4caf-8286-29dea98fc2e2" ,"89e105c9-2c45-42a9-a5f0-fc1ea6e7ba8b"]}'