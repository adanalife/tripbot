package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeFollowWebhookValidPayload(t *testing.T) {
	payload := `{"data":[{"from_id":"123","from_name":"alice","to_id":"456","to_name":"channel","followed_at":"2024-01-01T00:00:00Z"}]}`
	req := httptest.NewRequest("POST", "/webhooks/twitch/users/follows", strings.NewReader(payload))

	resp, err := decodeFollowWebhookResponse(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data.Follows) != 1 {
		t.Fatalf("expected 1 follow, got %d", len(resp.Data.Follows))
	}
	if resp.Data.Follows[0].FromName != "alice" {
		t.Fatalf("got from_name %q, want %q", resp.Data.Follows[0].FromName, "alice")
	}
}

func TestDecodeFollowWebhookEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/webhooks/twitch/users/follows", strings.NewReader(""))
	resp, err := decodeFollowWebhookResponse(req)
	if err != nil {
		t.Fatalf("unexpected error on empty body: %v", err)
	}
	if len(resp.Data.Follows) != 0 {
		t.Fatalf("expected 0 follows, got %d", len(resp.Data.Follows))
	}
}

func TestDecodeSubscriptionWebhookValidPayload(t *testing.T) {
	payload := `{"data":[{"id":"abc","event_data":{"broadcaster_id":"225469317","broadcaster_name":"ADanaLife_","is_gift":false,"plan_name":"Channel Subscription","tier":"1000","user_id":"26784661","user_name":"MathGaming"}}]}`
	req := httptest.NewRequest("POST", "/webhooks/twitch/subscriptions/events", strings.NewReader(payload))

	resp, err := decodeSubscriptionWebhookResponse(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Data.Events))
	}
	if resp.Data.Events[0].Subscription.UserName != "MathGaming" {
		t.Fatalf("got user_name %q, want %q", resp.Data.Events[0].Subscription.UserName, "MathGaming")
	}
	if resp.Data.Events[0].Id != "abc" {
		t.Fatalf("got id %q, want %q", resp.Data.Events[0].Id, "abc")
	}
}

func TestDecodeSubscriptionWebhookEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/webhooks/twitch/subscriptions/events", strings.NewReader(""))
	resp, err := decodeSubscriptionWebhookResponse(req)
	if err != nil {
		t.Fatalf("unexpected error on empty body: %v", err)
	}
	if len(resp.Data.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(resp.Data.Events))
	}
}
