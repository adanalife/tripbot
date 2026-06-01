package twitch

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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
//
// account names the identity the call authorized against ("bot" |
// "broadcaster"); on a 401 it triggers Reauth so the stale user-access-token
// is re-established from the DB and the next call uses a fresh one. Pass ""
// to opt out — for app-access-token calls (getChannelID's GetUsers) and the
// mid-bootstrap GetUsers, where re-reading a user token wouldn't help.
func (cl *API) checkHelixResp(ctx context.Context, endpoint, account string, resp *helix.ResponseCommon) bool {
	if resp == nil || resp.StatusCode < 400 {
		return false
	}
	slog.ErrorContext(ctx, fmt.Sprintf("helix %s returned %d: %s", endpoint, resp.StatusCode, resp.ErrorMessage))
	instrumentation.TwitchHelixErrors.Inc(endpoint, resp.StatusCode)
	if resp.StatusCode == http.StatusUnauthorized && account != "" {
		cl.Reauth(ctx, account)
	}
	return true
}
