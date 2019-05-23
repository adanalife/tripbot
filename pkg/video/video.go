package video

import (
	"errors"
	"fmt"
	"path"
	"regexp"

	"github.com/dmerrick/danalol-stream/pkg/config"
)

func init() {
	var validID = regexp.MustCompile(`^[a-z]+\[[0-9]+\]$`)
}

// a DashStr is the string we get from the dashcam
// an example file: 2018_0514_224801_013.MP4
// an example dashstr: 2018_0514_224801_013
type Video struct {
	DashStr string
}

func New(file string) Video {
	dashStr := removeFileExtension(path.Base(videoPath))
	if !valid(dashStr) {
		return errors.New("did not match regex")
	}
	return video.Video{file}
}

// ex: 2018_0514_224801_013
func (v Video) String() string {
	fmt.Println()
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

func valid(dashStr) bool {
	return validID.MatchString(dashStr[:21])
}

func removeFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}
