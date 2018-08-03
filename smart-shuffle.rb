#!/usr/bin/env ruby
# this ugly script generates a playlist for FFMPEG

# use the dir passed in via CLI
vid_dir = ARGV.shift

# find all the files in the dir
all_files = Dir.glob("#{vid_dir}/**/*")
# but only incude MP4 files
files = all_files.select{|f| f =~ /\.MP4$/ }

# create a hash that organizes files by date taken
by_day = {}
files.each do |file|
  date = file.sub(/.*\//,'')[0..8]
  by_day[date] ||= []
  by_day[date] << file
end

# since some files are in a "read-only" subdir, make sure
# those are in the correct order when sorted
by_day.each do |date, file_list|
  file_list.sort! do |a,b|
    a.sub(/\/RO/,'') <=> b.sub(/\/RO/,'')
  end
end

# print the formatted output for ffmpeg
by_day.keys.shuffle.each do |day|
  by_day[day].each do |file|
    puts "file '#{file}'"
  end
end
