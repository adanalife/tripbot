INRES="1920x1080" # input resolution
OUTRES="1920x1080" # output resolution
FPS="30" # target FPS
GOP="60" # i-frame interval, should be double of FPS, 
GOPMIN="30" # min i-frame interval, should be equal to fps, 
THREADS="2" # max 6
CBR="1000k" # constant bitrate (should be between 1000k - 3000k)
QUALITY="ultrafast"  # one of the many FFMPEG preset
AUDIO_RATE="44100"
STREAM_KEY="$1" # use the terminal command Streaming streamkeyhere to stream your video to twitch or justin
SERVER="live-dfw" # twitch server, see https://stream.twitch.tv/ingests/ for list

FFMPEG_PATH="/home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/bin/ffmpeg"

# $FFMPEG_PATH -f x11grab -s "$INRES" -r "$FPS" -i :0.0 -f pulse -i 0 -f flv -ac 2 -ar $AUDIO_RATE \
#  -vcodec libx264 -g $GOP -keyint_min $GOPMIN -b:v $CBR -minrate $CBR -maxrate $CBR -pix_fmt yuv420p\
#  -s $OUTRES -preset $QUALITY -tune film -acodec aac -threads $THREADS -strict normal \
$FFMPEG_PATH -f x11grab -s "$INRES" -r "$FPS" -i :0.0 -i /home/dmerrick/Videos/tester.mp4 -f flv -ac 2 -ar $AUDIO_RATE \
  -vcodec h264_nvenc -g $GOP -keyint_min $GOPMIN -b:v $CBR -minrate $CBR -maxrate $CBR -pix_fmt yuv420p\
  -s $OUTRES -tune film -acodec aac -threads $THREADS -strict normal \
  -bufsize $CBR "rtmp://$SERVER.twitch.tv/app/$STREAM_KEY"
