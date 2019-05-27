#!/usr/bin/env ruby

# this script manages the text file that OBS reads from
# it is used to update the rotating text in the corner

sub_goal = "2/10"
leaderboard_file = "./OBS/leaderboard-copy.txt"
message_file = "./OBS/left-message.txt"

possible_messages = [
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Type !help in chat for instructions",
  "Miles have been moved over to a new system!",
  "Miles have been moved over to a new system!",
  "OliveCat50 and tadeas14 are miles superstars",
  "Sub goal #1 reached! Emotes coming soon",
  "Sub goal #2 (map overlay): #{sub_goal}",
  "Your subscriptions keep the channel running",
  "Earn miles for every minute you watch (!miles)",
  "I won't be offended if you play your own music",
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
    begin
      miles, leader = File.read(File.expand_path(leaderboard_file)).split("\n")[1].split(":")
    rescue
      next
    end
    next unless leader
    current = "#{leader.strip} is leader with #{miles} (!leaderboard)"
  end

  File.write(File.expand_path(message_file), current)
  sleep 45
end

