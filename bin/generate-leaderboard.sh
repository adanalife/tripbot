#!/usr/bin/env bash

cp tripbot.db tripbot-copy.db

go run tripbot4000-leaderboard.go > OBS/leaderboard.txt


