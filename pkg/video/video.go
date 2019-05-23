package video

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/config"
)

// a DashStr is the string we get from the dashcam
// an example file: 2018_0514_224801_013.MP4
// an example dashstr: 2018_0514_224801_013
type Video struct {
	DashStr string
}

func New(file string) (Video, error) {
	var newVid Video

	if file == "" {
		return newVid, errors.New("no file provided")
	}
	fileName := path.Base(file)
	dashStr := removeFileExtension(fileName)

	// validate the dash string
	err := valid(dashStr)
	if err != nil {
		return newVid, err
	}
	newVid = Video{dashStr}
	return newVid, err
}

// ex: 2018_0514_224801_013
func (v Video) String() string {
	return v.DashStr
}

// ex: 2018_0514_224801_013.MP4
func (v Video) File() string {
	return fmt.Sprintf("%s.MP4", v.DashStr)
}

// ex: /Volumes/.../2018_0514_224801_013.MP4
func (v Video) Path() string {
	return path.Join(config.VideoDir, v.File())
}

func valid(dashStr string) error {
	if len(dashStr) < 20 {
		return errors.New("dash string too short")
	}
	shortened := dashStr[:20]

	if strings.HasPrefix(".", shortened) {
		return errors.New("dash string can't be a hidden file")
	}

	//TODO: this should probably live in an init()
	var validDashStr = regexp.MustCompile(`^[_0-9]{20}$`)
	if !validDashStr.MatchString(shortened) {
		return errors.New("dash string did not match regex")
	}
	return nil
}

func removeFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}
