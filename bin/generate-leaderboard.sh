#!/usr/bin/env bash

# this script manages the text file that OBS reads
# most of the time the text file will be empty, but
# occasionally it will contain the miles leaderboard

echo "starting script"

# trap ctrl-c and call ctrl_c()
trap ctrl_c INT

function ctrl_c() {
	echo "** Caught CTRL-C"
	exit 1
}

while true; do
	if (( RANDOM % 8 == 0 )); then
		go run cmd/leaderboard/leaderboard.go > OBS/leaderboard.txt 2>/dev/null
		# make a copy so we can read it in the rotater script
		cp OBS/leaderboard.txt OBS/leaderboard-copy.txt
	else
		# clear out the file
		echo "" > OBS/leaderboard.txt
	fi
	sleep 30
done

