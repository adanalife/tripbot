package twitch

import (
	"testing"

	"github.com/nicklaw5/helix/v2"
)

func TestCheckHelixResp_NilResponse(t *testing.T) {
	if checkHelixResp("GetUsers", nil) {
		t.Fatal("nil response should not be treated as an error")
	}
}

func TestCheckHelixResp_Success(t *testing.T) {
	rc := &helix.ResponseCommon{StatusCode: 200}
	if checkHelixResp("GetUsers", rc) {
		t.Fatal("200 should not be treated as an error")
	}
}

func TestCheckHelixResp_NoContent(t *testing.T) {
	// 204 is still success — guard against >= 400, not != 200
	rc := &helix.ResponseCommon{StatusCode: 204}
	if checkHelixResp("GetUsers", rc) {
		t.Fatal("204 should not be treated as an error")
	}
}

func TestCheckHelixResp_ClientError(t *testing.T) {
	// 403 was the concrete 2026-05-15 incident: bot lost mod scope on the
	// channel, GetChannelChatChatters returned 403 with empty Data.
	rc := &helix.ResponseCommon{StatusCode: 403, ErrorMessage: "insufficient scope"}
	if !checkHelixResp("GetChannelChatChatters", rc) {
		t.Fatal("403 should be treated as an error")
	}
}

func TestCheckHelixResp_ServerError(t *testing.T) {
	rc := &helix.ResponseCommon{StatusCode: 503, ErrorMessage: "unavailable"}
	if !checkHelixResp("GetSubscriptions", rc) {
		t.Fatal("5xx should be treated as an error")
	}
}
