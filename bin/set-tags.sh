#!/usr/bin/env bash

# this script sets the stream tags
#TODO: turn this into golang code

# 6ea6bca4-4712-4ab9-a906-e3336a9d8039 english (not required)
# 1400ca9c-84ea-414e-a85b-076a70d38ecf automotive
# 77223888-8535-4614-974b-b1b2673456eb visual asmr
# a4fac2cc-7cd4-44a6-b620-178182389a5b exploration
# a6ff589a-33e5-4caf-8286-29dea98fc2e2 travel
# 89e105c9-2c45-42a9-a5f0-fc1ea6e7ba8b outdoors

if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 [server-url]"
  exit 0
fi

SERVER_URL=$1

RESP=$(curl -s "$SERVER_URL/auth/twitch?auth=yes")
echo $RESP | jq

# extract the fields from the response
CHANNEL_ID=$(echo $RESP | jq -r .channel_id)
CLIENT_ID=$(echo $RESP | jq -r .client_id)
USER_ACCESS_TOKEN=$(echo $RESP | jq -r .user_access_token)

curl -s -H "Client-ID: $CLIENT_ID" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $USER_ACCESS_TOKEN" \
  -X PUT "https://api.twitch.tv/helix/streams/tags?broadcaster_id=$CHANNEL_ID" \
  -d '{"tag_ids": ["1400ca9c-84ea-414e-a85b-076a70d38ecf","77223888-8535-4614-974b-b1b2673456eb","a4fac2cc-7cd4-44a6-b620-178182389a5b","a6ff589a-33e5-4caf-8286-29dea98fc2e2" ,"89e105c9-2c45-42a9-a5f0-fc1ea6e7ba8b"]}'
