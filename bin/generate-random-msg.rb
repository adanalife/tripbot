#!/usr/bin/env ruby

# this script manages the text file that OBS reads from
# it is used to update the rotating text in the corner

sub_goal = "4/5"
leaderboard_file = "./OBS/leaderboard-copy.txt"
message_file = "./OBS/current-message.txt"

# fetch the current leader
miles, leader = File.read(File.expand_path(leaderboard_file)).split("\n")[1].split(":")

possible_messages = [
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Sub goal #1 (emotes): #{sub_goal}",
  "Please leave feedback in the comments <3",
  "Subscribe with Twitch Prime <3",
  "Choppy? Sorry, new hardware coming soon",
  "Use !less to mark something as boring",
  "Earn 1 mile for every 10m watched !miles",
  "Don't like music? Mute it!",
  "Music by Soma.FM !song",
  "Tripbot loves you <3 !tripbot",
  "See something cool? Clip it!",
  "#{leader.strip} is leader with #{miles}"
]

puts "starting #$PROGRAM_NAME script"

loop do
  File.write(File.expand_path(message_file), possible_messages.sample)
  sleep 45
end

