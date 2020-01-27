#!/usr/bin/env bash

#TODO: set volume
#TODO: better errors if mopidy is down


# get all playlists
#curl -X POST -H Content-Type:application/json -d '{
#  "method": "core.playlists.get_playlists",
#  "jsonrpc": "2.0",
#  "params": {
#    "include_tracks": null
#  },                    
#  "id": 1
#}' http://127.0.0.1:6680/mopidy/rpc


# i think we can delete this
#curl -X POST -H Content-Type:application/json -d '{
#  "method": "core.playlists.refresh",
#  "jsonrpc": "2.0",
#  "params": {
#    "uri_scheme": null
#  },
#  "id": 1
#}' http://127.0.0.1:6680/mopidy/rpc


#TODO: fetch URI from previous commadn
echo "getting playlist items"
curl -s -X POST -H Content-Type:application/json -d '{
  "method": "core.playlists.get_items",
  "jsonrpc": "2.0",
  "params": {
    "uri": "m3u:Groove%20Salad.m3u8"
  },
  "id": 1
}' http://127.0.0.1:6680/mopidy/rpc | jq
echo


#TODO: fetch URI from previous command
echo "adding to tracklist"
curl -s -X POST -H Content-Type:application/json -d '{
  "method": "core.tracklist.add",
  "jsonrpc": "2.0",
  "params": {
    "uri": "http://somafm.com/groovesalad256.pls"
  },
  "id": 1
}' http://127.0.0.1:6680/mopidy/rpc | jq
echo

# print tracks in playlist
#curl -X POST -H Content-Type:application/json -d '{
#  "method": "core.tracklist.get_tracks",
#  "jsonrpc": "2.0",
#  "params": {},
#  "id": 1
#}' http://127.0.0.1:6680/mopidy/rpc

# start teh song
echo "starting the track"
curl -s -X POST -H Content-Type:application/json -d '{
  "method": "core.playback.play",
  "jsonrpc": "2.0",
  "params": {
    "tl_track": null,
    "tlid": null
  },
  "id": 1
}' http://127.0.0.1:6680/mopidy/rpc | jq
