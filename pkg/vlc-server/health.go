package vlcServer

import (
	"fmt"
	"net/http"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
)

func healthCheck(w http.ResponseWriter) {
	obsPid := helpers.ReadPidFile(config.OBSPidFile)
	pidRunning, err := helpers.PidExists(obsPid)
	if err != nil {
		terrors.Log(err, "error fetching OBS pid")
		http.Error(w, "error fetching OBS pid", http.StatusFailedDependency)
		return
	}
	if !pidRunning {
		http.Error(w, "OBS not running", http.StatusFailedDependency)
		return
	}
	fmt.Fprintf(w, "OK")
}
