package chatbot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/gateway"
)

// recordingYouTubeSend records send calls so the flag-dispatch test can assert
// which path a send took.
type recordingYouTubeSend struct {
	calls  int
	chatID string
	text   string
}

func (r *recordingYouTubeSend) send(_ context.Context, chatID, text string) error {
	r.calls++
	r.chatID = chatID
	r.text = text
	return nil
}

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

func TestFlaggedYouTubeSend_DispatchesOnFlag(t *testing.T) {
	gw := &recordingYouTubeSend{}
	inproc := &recordingYouTubeSend{}

	flagOn := feature.NewInMemoryClient(map[string]feature.Flag{
		YouTubeGatewayFlagKey: {Key: YouTubeGatewayFlagKey, Enabled: true},
	})
	flagOff := feature.NewInMemoryClient(nil) // unknown key → off

	// flag on → gateway
	on := flaggedYouTubeSend{app: &App{Flags: flagOn}, gateway: gw, inproc: inproc}
	if err := on.send(context.Background(), "c1", "hi"); err != nil {
		t.Fatal(err)
	}
	if gw.calls != 1 || inproc.calls != 0 {
		t.Errorf("flag on should route to the gateway; gw=%d inproc=%d", gw.calls, inproc.calls)
	}

	// flag off → in-process (the default until toggled)
	off := flaggedYouTubeSend{app: &App{Flags: flagOff}, gateway: gw, inproc: inproc}
	if err := off.send(context.Background(), "c1", "hi"); err != nil {
		t.Fatal(err)
	}
	if inproc.calls != 1 {
		t.Errorf("flag off should route in-process; inproc=%d", inproc.calls)
	}
}

func TestNewYouTubeSend_NoURLSkipsGatewayWrapper(t *testing.T) {
	// With no YOUTUBE_API_URL there's nothing to flag — it's the plain in-process
	// adapter, not a flaggedYouTubeSend wrapper.
	if _, ok := newYouTubeSend(&App{}).(realYouTubeSend); !ok {
		t.Error("expected realYouTubeSend when YOUTUBE_API_URL is empty")
	}
}
