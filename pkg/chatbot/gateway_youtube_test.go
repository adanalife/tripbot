package chatbot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/gateway"
)

func TestGatewayYouTubeSend_PostsToChat(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// chatID is deliberately ignored — the gateway resolves the active chat.
	if err := (gatewayYouTubeSend{client: gateway.New(srv.URL)}).
		send(context.Background(), "ignored-chat-id", "hello"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat" {
		t.Errorf("path = %q, want /v1/chat", gotPath)
	}
	// identity is "" (gateway default) and the chatID is not sent.
	if !strings.Contains(gotBody, `"text":"hello"`) || !strings.Contains(gotBody, `"identity":""`) {
		t.Errorf("body = %q, want text=hello identity=empty", gotBody)
	}
	if strings.Contains(gotBody, "ignored-chat-id") {
		t.Errorf("body should not carry the chatID; got %q", gotBody)
	}
}

func TestGatewayYouTubeSend_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if err := (gatewayYouTubeSend{client: gateway.New(srv.URL)}).
		send(context.Background(), "", "hi"); err == nil {
		t.Error("expected an error on a 502 from the gateway")
	}
}

func TestNewYouTubeSend_NoURLUsesInProcess(t *testing.T) {
	// With no YOUTUBE_API_URL there's no gateway to reach — fall back to the
	// in-process insert.
	prev := c.Conf.YouTubeAPIURL
	c.Conf.YouTubeAPIURL = ""
	defer func() { c.Conf.YouTubeAPIURL = prev }()

	if _, ok := newYouTubeSend().(realYouTubeSend); !ok {
		t.Error("expected realYouTubeSend when YOUTUBE_API_URL is empty")
	}
}

func TestNewYouTubeSend_WithURLUsesGateway(t *testing.T) {
	// A wired instance routes through the gateway unconditionally — no flag.
	prev := c.Conf.YouTubeAPIURL
	c.Conf.YouTubeAPIURL = "http://gateway-youtube:8080"
	defer func() { c.Conf.YouTubeAPIURL = prev }()

	if _, ok := newYouTubeSend().(gatewayYouTubeSend); !ok {
		t.Error("expected gatewayYouTubeSend when YOUTUBE_API_URL is set")
	}
}
