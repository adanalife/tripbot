package main

import (
	// c "github.com/adanalife/tripbot/pkg/config/tripbot"
	// terrors "github.com/adanalife/tripbot/pkg/errors"
	"log"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/davecgh/go-spew/spew"
)

func main() {
	name1 := "danatest"
	log.Println("creating score", name1)
	_, err := scoreboards.Create(name1)
	// spew.Dump(err)
	// spew.Dump(s)

	name2 := "danatest" + helpers.RandString(4)
	log.Println("creating score", name2)
	_, err = scoreboards.Create(name2)
	spew.Dump(err)
	// spew.Dump(s)

	log.Println("looking up mathgaming")
	u := users.Find("mathgaming")
	spew.Dump(u)

	log.Println("getting score for", name1)
	spew.Dump(u.GetScore(name1))
	log.Println("getting score for", name2)
	spew.Dump(u.GetScore(name2))

	log.Println("adding to score", name1)
	u.AddToScore(name1, 1.1)

	log.Println("getting score for", name1)
	spew.Dump(u.GetScore(name1))
}
