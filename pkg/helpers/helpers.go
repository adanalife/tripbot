package helpers

import (
	"context"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/store"
)

func CreateOrFindInContext() store.Store {
	datastore := context.Background().Value(helpers.StoreKey)
	if datastore != nil {
		return datastore
	} else {
		datastore := store.NewStore()
		datastore.Open()
		return datastore
	}
}
