package audio

import (
	"fmt"
	"log"
	"runtime"
	"syscall"

	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/fhs/gompd/mpd"
	"github.com/mitchellh/go-ps"
	"github.com/skratchdot/open-golang/open"
)

var mpdConn *mpd.Client
var Enabled = true

const (
	grooveSaladURL = "http://somafm.com/groovesalad256.pls"
	mpdServer      = "localhost:6600"
)

func init() {
	// disable audio on OS X
	if runtime.GOOS != "linux" {
		log.Println("Disabling audio since we're not on Linux")
		Enabled = false
		return
	}

	// connect to the MPD server
	connect()

	if !config.DisableMusicAutoplay {
		//TODO: this shouldn't live in init probably
		PlayGrooveSalad()
	}
}

func connect() {
	var err error
	// Connect to MPD server
	mpdConn, err = mpd.Dial("tcp", mpdServer)
	if err != nil {
		terrors.Log(err, "Error connecting to MPD")
	}
}

func mpdState() string {
	status, err := mpdConn.Status()
	if err != nil {
		terrors.Log(err, "Error getting MPD status")
		return "error"
	}
	return status["state"]
}

func PlayGrooveSalad() {
	log.Println("Starting Groove Salad")
	err := mpdConn.Add(grooveSaladURL)
	if err != nil {
		terrors.Log(err, "Error adding to MPD playlist")
	}
	// negative values play the current track
	err = mpdConn.Play(-1)
	if err != nil {
		terrors.Log(err, "Error playing MPD track")
	}
}

// RefreshClient is intended to run periodically, because otherwise
// the connection will time out
func RefreshClient() {
	if Enabled {
		state := mpdState()
		if state == "error" {
			connect()
		}
	}
}

func CurrentlyPlaying() string {
	output := ""
	if Enabled {
		state := mpdState()
		if state != "play" {
			output = fmt.Sprintf("Player state: %s", state)
			return output
		}
		song, err := mpdConn.CurrentSong()
		if err != nil {
			terrors.Log(err, "Error getting current song from MPD")
			return ""
		}
		//TODO: there are other attributes in here, use them?
		output = song["Title"]
	}
	return output
}

func Shutdown() {
	if Enabled {
		err := mpdConn.Close()
		if err != nil {
			terrors.Log(err, "Error while closing MPD connection")
		}
	}

}

func RestartItunes() {
	stopiTunes()
	startiTunes()
}

func stopiTunes() {
	itunesBinary := "iTunes"

	processes, err := ps.Processes()
	if err != nil {
		terrors.Log(err, "error getting pids")
	}

	//spew.Dump(processes)

	var itunesProcess ps.Process
	for _, p := range processes {
		if p.Executable() == itunesBinary {
			itunesProcess = p
			// there probably isn't a second iTunes process
			break
		}
	}

	if itunesProcess != nil {
		log.Printf("pid for iTunes is %d, killing it...", itunesProcess.Pid())
		err = syscall.Kill(itunesProcess.Pid(), syscall.SIGKILL)
		if err != nil {
			terrors.Log(err, "error killing pid")
		}
	} else {
		log.Println("no iTunes process found")
	}
}

func startiTunes() {
	log.Println("opening iTunes")
	err := open.RunWith("http://somafm.com/groovesalad256.pls", "iTunes")
	if err != nil {
		terrors.Log(err, "error starting iTunes")
	}
}
