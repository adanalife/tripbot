#!/usr/bin/env ruby

sub_goal = "2/5"
leaderboard_file = "./OBS/leaderboard-copy.txt"
message_file = "./OBS/current-message.txt"

# fetch the current leader
miles, leader = File.read(File.expand_path(leaderboard_file)).split("\n")[1].split(":")

possible_messages = [
  "Sub goal #1 (emotes): #{sub_goal}",
  "Please leave feedback in the comments <3",
  "Subscribe with Twitch Prime <3",
  "Type !help in chat for instructions",
  "Use !less to mark something as boring",
  "Earn 1 mile for every 10m watched",
  "Don't like music? Mute it!",
  "Music by Soma.FM",
  "Something cool? Clip it!",
  "#{leader.strip} is leader with #{miles}"
]

puts "starting #$PROGRAM_NAME script"

loop do
  File.write(File.expand_path(message_file), possible_messages.sample)
  sleep 30
end

