package onscreensServer

import (
	"fmt"
	"html"
	"strings"

	rot "github.com/adanalife/tripbot/pkg/rotator"
	"github.com/adanalife/tripbot/pkg/weather"
)

// weatherView renders the clip's conditions as the weather overlay: the
// condition's icon beside the temperature, from the location feed's cached
// "Partly cloudy, 71°F". Hidden while there is no fresh reading, which is also
// what a clip with no GPS fix or a stopped feed leaves behind. A description
// without an icon shows whole, as text.
func weatherView(vars rot.Vars) onscreenView {
	desc := vars["weather"]
	if desc == "" {
		return onscreenView{}
	}
	icon := weather.Icon(desc)
	_, temp, found := strings.Cut(desc, ", ")
	if icon == "" || !found {
		return onscreenView{Content: html.EscapeString(desc), IsShowing: true}
	}
	return onscreenView{
		Content:   fmt.Sprintf(`<span class="wx-icon">%s</span> %s`, icon, html.EscapeString(temp)),
		IsShowing: true,
	}
}
