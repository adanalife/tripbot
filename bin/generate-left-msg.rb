#!/usr/bin/env ruby

# this script manages the text file that OBS reads from
# it is used to update the rotating text in the corner

sub_goal = "6/10"
donation_goal = "10/150"

leaderboard_file = "./OBS/leaderboard-copy.txt"
message_file = "./OBS/left-message.txt"

possible_messages = [
  "Type !help in chat for tips",
  "Type !help in chat for tips",
  "Sub goal #1 reached! Emotes coming soon",
  "Sub goal #2 (map overlay): #{sub_goal}",
  "Donation goal (!temperature): #{donation_goal}",
  "Twitch Prime subs keep us on air ;)",
  "Earn miles for every minute you watch (!miles)",
  "I won't be offended if you play your own music",
  "Music by Soma.fm (!song)",
  "Use !report to report stream issues",
  # "Tripbot loves you <3 (!tripbot)",
  "Where are we? (!location)",
  "LEADER",
  "RARE",
]

puts "starting #$PROGRAM_NAME script"

loop do
  # pick a random message
  current = possible_messages.sample

  # just for fun: a very rare message
  if current == "RARE"
    next unless rand(1000).zero?
    current = "You found the rare message! Make a clip for a prize!"
  end

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

