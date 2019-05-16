package config

// this is used to access the db in the context
//TODO: this probably doesnt belong here?
type storeContextKey string

var StoreKey = storeContextKey("store")

// https://twitchinsights.net/bots
var IgnoredUsers = []string{
	"adanalife_",
	"tripbot4000",
	"nightbot",
	"anotherttvviewer",
	"apricotdrupefruit",
	"commanderroot",
	"communityshowcase",
	"electricallongboard",
	"logviewer",
	"lurxx",
	"p0lizei_",
	"slocool",
	"unixchat",
	"v_and_k",
	"virgoproz",
	"zanekyber",
	"feuerwehr",
	"jobi_essen",
	"freddyybot",
	"taormina2600",
	"avocadobadado",
	"eubyt",
}
