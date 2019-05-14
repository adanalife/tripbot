#!/usr/bin/env ruby


require 'sinatra'

set :bind, '0.0.0.0'
set :port, 9090


get '/?' do
  redirect 'http://whereisdana.today'
end

get '/date' do
  'hello, world'
end
