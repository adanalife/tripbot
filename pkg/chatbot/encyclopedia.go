package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/wikipedia"
)

// Encyclopedia is the subset of article lookup chatbot commands depend on
// (just a one-line description of a place, for !town). Tests inject
// noopEncyclopedia; production uses pkg/wikipedia's keyless REST client.
type Encyclopedia interface {
	// Summary returns a sentence describing the article at title, or
	// wikipedia.ErrNoArticle when there is nothing to say about it.
	Summary(ctx context.Context, title string) (string, error)
}

// realEncyclopedia is the production Encyclopedia.
var realEncyclopedia = wikipedia.REST{}
