#!/usr/bin/env ruby

# this script manages the text file that OBS reads from
# it is used to update the rotating text in the corner

sub_goal = "4/5"
leaderboard_file = "./OBS/leaderboard-copy.txt"
message_file = "./OBS/current-message.txt"

possible_messages = [
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Sub goal #1 (emotes): #{sub_goal}",
  "Leave feedback in chat",
  "Subscribe with Twitch Prime <3",
  "Earn 1 mile for every 10m watched (!miles)",
  "I won't be offended if you mute the music",
  "Music by Soma.fm (!song)",
  "Tripbot loves you <3 (!tripbot)",
  "See something cool? Clip it!",
  "LEADER",
]

puts "starting #$PROGRAM_NAME script"

loop do
  # pick a random message
  current = possible_messages.sample

  if current == "LEADER"
    # fetch the current leader
    miles, leader = File.read(File.expand_path(leaderboard_file)).split("\n")[1].split(":")
    current = "#{leader.strip} is leader with #{miles} (!leaderboard)"
  end

  File.write(File.expand_path(message_file), current)
  sleep 45
end

