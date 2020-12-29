package main

import (
	// c "github.com/adanalife/tripbot/pkg/config/tripbot"
	// terrors "github.com/adanalife/tripbot/pkg/errors"
	"log"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/davecgh/go-spew/spew"
	"github.com/logrusorgru/aurora"
)

func main() {
	terrors.Initialize(c.Conf)

	name1 := "danatest"
	log.Println(aurora.Green("creating score " + name1))
	_, err := scoreboards.Create(name1)
	// spew.Dump(err)
	// spew.Dump(s)

	name2 := "danatest" + helpers.RandString(4)
	log.Println(aurora.Green("creating score " + name2))
	_, err = scoreboards.Create(name2)
	spew.Dump(err)
	// spew.Dump(s)

	log.Println(aurora.Green("looking up mathgaming"))
	u := users.Find("mathgaming")
	spew.Dump(u)

	log.Println(aurora.Green("getting score for " + name1))
	spew.Dump(u.GetScore(name1))
	log.Println(aurora.Green("getting score for " + name2))
	spew.Dump(u.GetScore(name2))

	log.Println(aurora.Green("adding to score " + name1))
	u.AddToScore(name1, 1.1)

	log.Println(aurora.Green("getting score for " + name1))
	spew.Dump(u.GetScore(name1))
}
