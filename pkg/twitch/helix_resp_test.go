package twitch

import (
	"context"
	"testing"

	"github.com/nicklaw5/helix/v2"
)

func TestCheckHelixResp_NilResponse(t *testing.T) {
	cl := New()
	if cl.checkHelixResp(context.Background(), "GetUsers", "", nil) {
		t.Fatal("nil response should not be treated as an error")
	}
}

func TestCheckHelixResp_Success(t *testing.T) {
	cl := New()
	rc := &helix.ResponseCommon{StatusCode: 200}
	if cl.checkHelixResp(context.Background(), "GetUsers", "", rc) {
		t.Fatal("200 should not be treated as an error")
	}
}

func TestCheckHelixResp_NoContent(t *testing.T) {
	cl := New()
	// 204 is still success — guard against >= 400, not != 200
	rc := &helix.ResponseCommon{StatusCode: 204}
	if cl.checkHelixResp(context.Background(), "GetUsers", "", rc) {
		t.Fatal("204 should not be treated as an error")
	}
}

func TestCheckHelixResp_ClientError(t *testing.T) {
	cl := New()
	// 403 was the concrete 2026-05-15 incident: bot lost mod scope on the
	// channel, GetChannelChatChatters returned 403 with empty Data. 403 is a
	// scope problem, not a stale token — no reauth, but still flagged.
	rc := &helix.ResponseCommon{StatusCode: 403, ErrorMessage: "insufficient scope"}
	if !cl.checkHelixResp(context.Background(), "GetChannelChatChatters", "bot", rc) {
		t.Fatal("403 should be treated as an error")
	}
}

func TestCheckHelixResp_ServerError(t *testing.T) {
	cl := New()
	rc := &helix.ResponseCommon{StatusCode: 503, ErrorMessage: "unavailable"}
	if !cl.checkHelixResp(context.Background(), "GetSubscriptions", "broadcaster", rc) {
		t.Fatal("5xx should be treated as an error")
	}
}

// A 401 with account="" must still report the error but must NOT trigger
// Reauth (which would make a live Twitch refresh call) — the app-token and
// mid-bootstrap callsites rely on this opt-out.
func TestCheckHelixResp_UnauthorizedNoAccountSkipsReauth(t *testing.T) {
	cl := New()
	rc := &helix.ResponseCommon{StatusCode: 401, ErrorMessage: "invalid oauth token"}
	if !cl.checkHelixResp(context.Background(), "GetUsers", "", rc) {
		t.Fatal("401 should be treated as an error")
	}
}
