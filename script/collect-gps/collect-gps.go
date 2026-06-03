package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/geo"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/natsclient"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	"github.com/adanalife/tripbot/pkg/video"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// this will hold the filename passed in via the CLI
var videoFile string
var current bool

func init() {
	//TODO: remove this if it's no longer needed
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }

	geo.SetDefault(geo.New(c.Conf.GoogleMapsAPIKey))

	flag.StringVar(&videoFile, "file", "", "File to load")
	flag.BoolVar(&current, "current", false, "Use currently-playing video")
	flag.Parse()

}

func main() {
	// set videoFile if -current was passed in
	if current {
		// first we check if too many flags were used
		if videoFile != "" {
			log.Fatal("you cannot use -current and -file at the same time")
		}
		// preload the currently-playing vid via a constructed Player (no
		// package-level defaultPlayer anymore).
		player := video.NewPlayer(
			onscreensClient.New(natsclient.DefaultPublisher(), c.Conf.Environment),
			vlcClient.New(c.Conf.VlcServerHost),
		)
		player.GetCurrentlyPlaying(context.Background())
		videoFile = player.Current().String()
	}

	// a file was passed in via the CLI
	if videoFile != "" {
		vid, err := video.LoadOrCreate(context.Background(), videoFile)
		if err != nil {
			slog.Error("unable to create video", "err", err, "file", videoFile)
		}
		lat, lon, err := vid.Location()
		if err != nil {
			log.Fatalf("failed to process image: %s", err.Error())
		}
		url := helpers.GoogleMapsURL(lat, lon)
		fmt.Println(url)

	} else {

		// loop over every file in the screencapDir
		err := filepath.Walk(c.Conf.VideoDir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// skip the directory name itself
				if path == c.Conf.VideoDir {
					return nil
				}

				// actually process the image
				vid, err := video.LoadOrCreate(context.Background(), path)
				if err != nil {
					slog.Error("unable to create video", "err", err, "path", path)
					return nil
				}
				lat, lon, err := vid.Location()
				if err != nil {
					slog.Error("failed to process video", "err", err, "path", path)
					return nil
				}
				url := helpers.GoogleMapsURL(lat, lon)
				fmt.Println(url)
				return err
			})
		// something went wrong walking the directory
		if err != nil {
			slog.Error("directory walk failed", "err", err)
		}
	}

}
