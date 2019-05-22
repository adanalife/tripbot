#!/usr/bin/env ruby

# this script manages the text file that OBS reads from
# it is used to update the rotating text in the corner

message_file = "./OBS/right-message.txt"

possible_messages = [
  "Don't forget to follow :)",
  "Try running !tripbot <3",
]

puts "starting #$PROGRAM_NAME script"

loop do
  # pick a random message
  current = possible_messages.sample

  File.write(File.expand_path(message_file), current)
  sleep 90
end

