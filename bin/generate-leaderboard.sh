#!/usr/bin/env bash

echo "starting script"

# trap ctrl-c and call ctrl_c()
trap ctrl_c INT

function ctrl_c() {
	echo "** Caught CTRL-C"
	exit 1
}

while true; do
	if (( RANDOM % 10 == 0 )); then
		cp tripbot.db tripbot-copy.db
		go run tripbot4000-leaderboard.go > OBS/leaderboard.txt
		# make a copy
		cp OBS/leaderboard.txt OBS/leaderboard-copy.txt
	else
		# clear out the file
		echo "" > OBS/leaderboard.txt
	fi
	sleep 30
done

