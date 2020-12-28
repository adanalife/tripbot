package main

import (
	// c "github.com/adanalife/tripbot/pkg/config/tripbot"
	// terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/davecgh/go-spew/spew"
)

func main() {
	s, err := scoreboards.Create("danatest")
	spew.Dump(err)
	spew.Dump(s)

	s, err = scoreboards.Create("danatest" + helpers.RandString(4))
	spew.Dump(err)
	spew.Dump(s)
}
