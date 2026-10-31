// Package contract is the anti-drift bridge between tripbot (the consumer and
// source of truth) and the infra/cdk8s manifests (the producer of k8s objects).
// It holds the canonical service names, ports, and env-var keys shared across
// the two repos as typed Go values, and emits them as contract.json via
// `go generate`. A sibling test asserts the committed contract.json matches
// these constants, so any drift fails CI here in tripbot.
//
// The committed pkg/contract/contract.json is the canonical copy; the infra
// side syncs FROM it (`task contract:sync`). Edit the constants below, run
// `go generate ./pkg/contract`, and commit the regenerated JSON together.
//
// Where tripbot already owns a value (the obs-websocket addr default, the
// env-var keys behind pkg/config/tripbot's envconfig tags), the constant here
// is cross-checked against that definition rather than being an independent
// literal. Values with no prior Go home (the logical service
// names, the various pod ports, the stream env keys read only by shell/docker)
// are declared here as their new canonical home.
//
// JSON key order is fixed (it matches the hand-authored infra contract.json):
// Current() returns ordered key/value slices, and Marshal renders them in that
// order so the generated file is stable and reviewable.
package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// comment is the leading "_comment" field stamped into contract.json. It tells
// a reader of the JSON which repo owns the file and how the two sides stay in
// sync.
const comment = "Anti-drift contract between tripbot (consumer, source of truth) and infra/cdk8s (producer). Canonical service names, ports, and env-var keys. Generated from Go constants in tripbot's pkg/contract via `go generate ./pkg/contract`; the infra side syncs from it via `task contract:sync`. Edit pkg/contract/contract.go and regenerate — do not hand-edit this file."

// Logical service names → their Kubernetes Service name. These are the names
// tripbot clients dial (PLAYOUT_HOST, ONSCREENS_SERVER_HOST, the
// obs-websocket addr) and the names cdk8s must stamp onto the matching
// Service objects.
const (
	// ServiceTripbot is the chatbot / admin-panel service.
	ServiceTripbot = "tripbot"
	// ServiceOnscreensServer is the onscreens-server (overlay render) service.
	ServiceOnscreensServer = "onscreens-server"
	// ServiceOBSTwitch is the OBS instance streaming to Twitch. This matches
	// the obs-websocket addr default baked into pkg/obs (obs-twitch:4455).
	ServiceOBSTwitch = "obs-twitch"
	// ServiceOBSYouTube is the OBS instance streaming to YouTube.
	ServiceOBSYouTube = "obs-youtube"
	// ServiceOBSTikTok, ServiceOBSFacebook, and ServiceOBSInstagram name the
	// OBS instances for the remaining platforms. Referenced as sibling
	// hostnames by tripbot's per-platform config even before those OBS deploys
	// exist (the tiktok/instagram scenes need a vertical canvas first).
	ServiceOBSTikTok    = "obs-tiktok"
	ServiceOBSFacebook  = "obs-facebook"
	ServiceOBSInstagram = "obs-instagram"
	// ServiceNATS is the NATS service every component dials for the event bus
	// and the command subjects. It lives in the <env>-platform namespace rather
	// than the app namespace, so callers build an FQDN around this name; the
	// namespace pattern is env topology and stays with each cdk8s layer.
	ServiceNATS = "nats"
	// ServicePostgres is the Postgres service (DATABASE_HOST in cluster).
	ServicePostgres = "postgres"
)

// Per-platform service names. Each streaming platform runs its own full stack
// (tripbot + playout + onscreens + obs); the names carry the platform suffix
// so a Service only ever selects its own platform's pods. obs has been
// per-platform since #629; the cdk8s app factory brings tripbot/onscreens onto
// the same shape. The bare ServiceTripbot/ServiceOnscreensServer above remain
// the app-identity prefixes (Secret/ConfigMap names) — only the workload
// Services carry the suffix.
//
// ServicePlayout* name playout's Services; the adanalife/playout repo authors
// them and reads these names back from the synced contract.
const (
	ServiceTripbotTwitch      = "tripbot-twitch"
	ServiceTripbotYouTube     = "tripbot-youtube"
	ServiceTripbotTikTok      = "tripbot-tiktok"
	ServiceTripbotFacebook    = "tripbot-facebook"
	ServiceTripbotInstagram   = "tripbot-instagram"
	ServiceOnscreensTwitch    = "onscreens-twitch"
	ServiceOnscreensYouTube   = "onscreens-youtube"
	ServiceOnscreensTikTok    = "onscreens-tiktok"
	ServiceOnscreensFacebook  = "onscreens-facebook"
	ServiceOnscreensInstagram = "onscreens-instagram"
	ServicePlayoutTwitch      = "playout-twitch"
	ServicePlayoutYouTube     = "playout-youtube"
	ServicePlayoutTikTok      = "playout-tiktok"
	ServicePlayoutFacebook    = "playout-facebook"
	ServicePlayoutInstagram   = "playout-instagram"
	// ServiceMediaMTX* name the per-platform RTSP relay between playout and
	// OBS: playout publishes rtsp://mediamtx-<platform>:8554/dashcam and that
	// platform's OBS pulls it. The infra repo's cdk8s authors the Services;
	// the names live here so a rename can't silently break the OBS pull.
	ServiceMediaMTXTwitch    = "mediamtx-twitch"
	ServiceMediaMTXYouTube   = "mediamtx-youtube"
	ServiceMediaMTXTikTok    = "mediamtx-tiktok"
	ServiceMediaMTXFacebook  = "mediamtx-facebook"
	ServiceMediaMTXInstagram = "mediamtx-instagram"
	// ServiceGateway* name the per-platform platform-gateway Services — the
	// only holder of each platform's API credential, and the host tripbot's
	// <PLATFORM>_API_URL dials for chat and Helix. The adanalife/platform-gateway
	// repo's cdk8s authors the Services; the names live here because tripbot's
	// own cdk8s, the gateway's cdk8s, and infra's Argo all have to agree on
	// them, and nothing else fails when they drift.
	ServiceGatewayTwitch    = "gateway-twitch"
	ServiceGatewayYouTube   = "gateway-youtube"
	ServiceGatewayTikTok    = "gateway-tiktok"
	ServiceGatewayFacebook  = "gateway-facebook"
	ServiceGatewayInstagram = "gateway-instagram"
)

// Pod ports. Several services co-locate on 8080 for their HTTP API but expose
// other ports (VNC, websocket, RTSP) on their own pods, so the keys are
// per-(service, role) rather than per-number.
const (
	// PortOBSVNC is the raw VNC port exposed by the OBS pods.
	PortOBSVNC = 5900
	// PortOBSWebsocket is the OBS WebSocket control port. Matches the
	// obs-twitch:4455 default in pkg/obs.
	PortOBSWebsocket = 4455
	// PortOBSNoVNC is the noVNC (browser VNC) port on the OBS pods.
	PortOBSNoVNC = 6080
	// PortOBSServer is the obs-server (Flask health/version/shutdown) port.
	PortOBSServer = 8080
	// PortPlayoutHTTP is the playback HTTP API port (playout's /playout/current).
	PortPlayoutHTTP = 8080
	// PortOnscreensHTTP is the onscreens-server HTTP API port.
	PortOnscreensHTTP = 8080
	// PortTripbotHTTP is the tripbot chatbot/admin HTTP port.
	PortTripbotHTTP = 8080
	// PortMediaMTXRTSP is the RTSP port on the MediaMTX relay pods — the port
	// playout publishes to and OBS reads from.
	PortMediaMTXRTSP = 8554
	// PortGatewayHTTP is the platform-gateway HTTP API port — the port behind
	// every <PLATFORM>_API_URL and the consent Ingress backend.
	PortGatewayHTTP = 8080
	// PortNATS is the NATS client port.
	PortNATS = 4222
	// PortPostgres is the Postgres port.
	PortPostgres = 5432
)

// Env-var keys shared between tripbot (which reads them) and cdk8s (which
// stamps them into ConfigMaps/Secrets). The host/server keys mirror the
// envconfig struct tags in pkg/config/tripbot/type.go and pkg/database; the
// obs-websocket key mirrors pkg/obs; the stream keys have no Go consumer
// (read only by the OBS image's shell entrypoint) and are owned here.
const (
	// EnvKeyOBSWebsocketAddr is the host:port pkg/obs dials for OBS control.
	EnvKeyOBSWebsocketAddr = "OBS_WEBSOCKET_ADDR"
	// EnvKeyOBSServerHost mirrors TripbotConfig.ObsServerHost.
	EnvKeyOBSServerHost = "OBS_SERVER_HOST"
	// EnvKeyPlayoutHost mirrors TripbotConfig.PlayoutHost.
	EnvKeyPlayoutHost = "PLAYOUT_HOST"
	// EnvKeyOnscreensServerHost mirrors TripbotConfig.OnscreensServerHost.
	EnvKeyOnscreensServerHost = "ONSCREENS_SERVER_HOST"
	// EnvKeyDatabaseHost is the Postgres host pkg/database requires.
	EnvKeyDatabaseHost = "DATABASE_HOST"
	// EnvKeyStreamPlatform selects which platform a per-platform instance
	// serves. Read by the OBS image entrypoint (which platform OBS streams to)
	// and by tripbot via TripbotConfig.Platform (which chat platform the bot
	// serves + its command surface) — one platform value per pipeline.
	EnvKeyStreamPlatform = "STREAM_PLATFORM"
	// EnvKeyStreamKey is the per-platform stream key. Read by the OBS image
	// entrypoint, not by Go.
	EnvKeyStreamKey = "STREAM_KEY"
)

// pair is one ordered key/value entry in a contract section.
type pair struct {
	Key   string
	Value any
}

// Contract holds the canonical contract sections in their on-disk order.
type Contract struct {
	Comment  string
	Services []pair
	Ports    []pair
	EnvKeys  []pair
}

// Current returns the contract built from the canonical Go constants, with the
// section keys in the same order as the hand-authored infra contract.json.
func Current() Contract {
	return Contract{
		Comment: comment,
		Services: []pair{
			{"tripbot", ServiceTripbot},
			{"onscreens_server", ServiceOnscreensServer},
			{"obs_twitch", ServiceOBSTwitch},
			{"obs_youtube", ServiceOBSYouTube},
			{"obs_tiktok", ServiceOBSTikTok},
			{"obs_facebook", ServiceOBSFacebook},
			{"obs_instagram", ServiceOBSInstagram},
			{"tripbot_twitch", ServiceTripbotTwitch},
			{"tripbot_youtube", ServiceTripbotYouTube},
			{"tripbot_tiktok", ServiceTripbotTikTok},
			{"tripbot_facebook", ServiceTripbotFacebook},
			{"tripbot_instagram", ServiceTripbotInstagram},
			{"playout_twitch", ServicePlayoutTwitch},
			{"playout_youtube", ServicePlayoutYouTube},
			{"playout_tiktok", ServicePlayoutTikTok},
			{"playout_facebook", ServicePlayoutFacebook},
			{"playout_instagram", ServicePlayoutInstagram},
			{"onscreens_twitch", ServiceOnscreensTwitch},
			{"onscreens_youtube", ServiceOnscreensYouTube},
			{"onscreens_tiktok", ServiceOnscreensTikTok},
			{"onscreens_facebook", ServiceOnscreensFacebook},
			{"onscreens_instagram", ServiceOnscreensInstagram},
			{"mediamtx_twitch", ServiceMediaMTXTwitch},
			{"mediamtx_youtube", ServiceMediaMTXYouTube},
			{"mediamtx_tiktok", ServiceMediaMTXTikTok},
			{"mediamtx_facebook", ServiceMediaMTXFacebook},
			{"mediamtx_instagram", ServiceMediaMTXInstagram},
			{"gateway_twitch", ServiceGatewayTwitch},
			{"gateway_youtube", ServiceGatewayYouTube},
			{"gateway_tiktok", ServiceGatewayTikTok},
			{"gateway_facebook", ServiceGatewayFacebook},
			{"gateway_instagram", ServiceGatewayInstagram},
			{"nats", ServiceNATS},
			{"postgres", ServicePostgres},
		},
		Ports: []pair{
			{"obs_vnc", PortOBSVNC},
			{"obs_websocket", PortOBSWebsocket},
			{"obs_novnc", PortOBSNoVNC},
			{"obs_server", PortOBSServer},
			{"playout_http", PortPlayoutHTTP},
			{"onscreens_http", PortOnscreensHTTP},
			{"tripbot_http", PortTripbotHTTP},
			{"mediamtx_rtsp", PortMediaMTXRTSP},
			{"gateway_http", PortGatewayHTTP},
			{"nats", PortNATS},
			{"postgres", PortPostgres},
		},
		EnvKeys: []pair{
			{"obs_websocket_addr", EnvKeyOBSWebsocketAddr},
			{"obs_server_host", EnvKeyOBSServerHost},
			{"playout_host", EnvKeyPlayoutHost},
			{"onscreens_server_host", EnvKeyOnscreensServerHost},
			{"database_host", EnvKeyDatabaseHost},
			{"stream_platform", EnvKeyStreamPlatform},
			{"stream_key", EnvKeyStreamKey},
		},
	}
}

// Marshal renders the contract as pretty-printed JSON with stable key order
// (2-space indent, trailing newline) — the exact bytes the generator writes to
// pkg/contract/contract.json and the test compares against.
func (c Contract) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")

	commentJSON, err := json.Marshal(c.Comment)
	if err != nil {
		return nil, fmt.Errorf("marshal _comment: %w", err)
	}
	fmt.Fprintf(&buf, "  %q: %s,\n", "_comment", commentJSON)

	sections := []struct {
		name  string
		pairs []pair
	}{
		{"services", c.Services},
		{"ports", c.Ports},
		{"env_keys", c.EnvKeys},
	}
	for i, section := range sections {
		fmt.Fprintf(&buf, "  %q: {\n", section.name)
		for j, p := range section.pairs {
			valJSON, err := json.Marshal(p.Value)
			if err != nil {
				return nil, fmt.Errorf("marshal %s.%s: %w", section.name, p.Key, err)
			}
			fmt.Fprintf(&buf, "    %q: %s", p.Key, valJSON)
			if j < len(section.pairs)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString("  }")
		if i < len(sections)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}

	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
