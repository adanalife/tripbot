package twitch

import (
	"fmt"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/nicklaw5/helix/v2"
)

// checkHelixResp returns true when the Twitch Helix API responded with a
// non-2xx status. The Helix SDK reports those as (resp, nil) with the
// response's Data field left empty — callers that only check err were
// treating them as success, which lets configuration bugs (bot missing
// moderator:read:chatters, expired token, wrong client_id) silently evict
// cached state.
//
// Concrete prior incident (2026-05-15): the bot lost mod on adanalife_staging,
// UpdateChatters got back a 403 with empty Data, the session map cleared
// every 61s, and !miles drifted to ~0 with no log or metric to point at.
//
// When this returns true, callers should log + early-return rather than
// trust resp.Data. endpoint is a short label used as the metric attribute
// (e.g. "GetUsers"); it should match the helix client method name.
func checkHelixResp(endpoint string, resp *helix.ResponseCommon) bool {
	if resp == nil || resp.StatusCode < 400 {
		return false
	}
	terrors.Log(nil, fmt.Sprintf("helix %s returned %d: %s", endpoint, resp.StatusCode, resp.ErrorMessage))
	instrumentation.TwitchHelixErrors.Inc(endpoint, resp.StatusCode)
	return true
}
