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
// VLC RTSP port, the env-var keys behind pkg/config/tripbot's envconfig tags),
// the constant here is cross-checked against that definition rather than being
// an independent literal. Values with no prior Go home (the logical service
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
// tripbot clients dial (VLC_SERVER_HOST, ONSCREENS_SERVER_HOST, the
// obs-websocket addr) and the names cdk8s must stamp onto the matching
// Service objects.
const (
	// ServiceTripbot is the chatbot / admin-panel service.
	ServiceTripbot = "tripbot"
	// ServiceVLCServer is the vlc-server (dashcam playback + RTSP) service.
	ServiceVLCServer = "vlc-server"
	// ServiceOnscreensServer is the onscreens-server (overlay render) service.
	ServiceOnscreensServer = "onscreens-server"
	// ServiceOBSTwitch is the OBS instance streaming to Twitch. This matches
	// the obs-websocket addr default baked into pkg/obs (obs-twitch:4455).
	ServiceOBSTwitch = "obs-twitch"
	// ServiceOBSYouTube is the OBS instance streaming to YouTube.
	ServiceOBSYouTube = "obs-youtube"
	// ServicePostgres is the Postgres service (DATABASE_HOST in cluster).
	ServicePostgres = "postgres"
)

// Per-platform service names. Each streaming platform runs its own full stack
// (tripbot + vlc + onscreens + obs); the names carry the platform suffix so a
// Service only ever selects its own platform's pods. obs has been per-platform
// since #629; the cdk8s app factory brings the other three onto the same shape.
// The bare ServiceTripbot/ServiceVLCServer/ServiceOnscreensServer above remain
// the app-identity prefixes (Secret/ConfigMap names) — only the workload
// Services carry the suffix.
const (
	ServiceTripbotTwitch    = "tripbot-twitch"
	ServiceTripbotYouTube   = "tripbot-youtube"
	ServiceVLCTwitch        = "vlc-twitch"
	ServiceVLCYouTube       = "vlc-youtube"
	ServiceOnscreensTwitch  = "onscreens-twitch"
	ServiceOnscreensYouTube = "onscreens-youtube"
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
	// PortVLCHTTP is the vlc-server HTTP API port.
	PortVLCHTTP = 8080
	// PortVLCOnscreensLegacy is the legacy onscreens port vlc-server used to
	// serve before onscreens-server split out.
	PortVLCOnscreensLegacy = 8081
	// PortVLCRTSP is the vlc-server RTSP output port. Matches the :8554 RTSP
	// chain baked into pkg/vlc-server.
	PortVLCRTSP = 8554
	// PortVLCVNC is the VNC port on the vlc-server pod.
	PortVLCVNC = 5900
	// PortOnscreensHTTP is the onscreens-server HTTP API port.
	PortOnscreensHTTP = 8080
	// PortTripbotHTTP is the tripbot chatbot/admin HTTP port.
	PortTripbotHTTP = 8080
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
	// EnvKeyVLCServerHost mirrors TripbotConfig.VlcServerHost.
	EnvKeyVLCServerHost = "VLC_SERVER_HOST"
	// EnvKeyOnscreensServerHost mirrors TripbotConfig.OnscreensServerHost.
	EnvKeyOnscreensServerHost = "ONSCREENS_SERVER_HOST"
	// EnvKeyDatabaseHost is the Postgres host pkg/database requires.
	EnvKeyDatabaseHost = "DATABASE_HOST"
	// EnvKeyStreamPlatform selects which platform OBS streams to. Read by the
	// OBS image entrypoint, not by Go.
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
			{"vlc_server", ServiceVLCServer},
			{"onscreens_server", ServiceOnscreensServer},
			{"obs_twitch", ServiceOBSTwitch},
			{"obs_youtube", ServiceOBSYouTube},
			{"tripbot_twitch", ServiceTripbotTwitch},
			{"tripbot_youtube", ServiceTripbotYouTube},
			{"vlc_twitch", ServiceVLCTwitch},
			{"vlc_youtube", ServiceVLCYouTube},
			{"onscreens_twitch", ServiceOnscreensTwitch},
			{"onscreens_youtube", ServiceOnscreensYouTube},
			{"postgres", ServicePostgres},
		},
		Ports: []pair{
			{"obs_vnc", PortOBSVNC},
			{"obs_websocket", PortOBSWebsocket},
			{"obs_novnc", PortOBSNoVNC},
			{"obs_server", PortOBSServer},
			{"vlc_http", PortVLCHTTP},
			{"vlc_onscreens_legacy", PortVLCOnscreensLegacy},
			{"vlc_rtsp", PortVLCRTSP},
			{"vlc_vnc", PortVLCVNC},
			{"onscreens_http", PortOnscreensHTTP},
			{"tripbot_http", PortTripbotHTTP},
			{"postgres", PortPostgres},
		},
		EnvKeys: []pair{
			{"obs_websocket_addr", EnvKeyOBSWebsocketAddr},
			{"obs_server_host", EnvKeyOBSServerHost},
			{"vlc_server_host", EnvKeyVLCServerHost},
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
