package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	rot "github.com/adanalife/tripbot/pkg/rotator"
	"github.com/gorilla/mux"
)

// RotatorStore is the persistence seam the /api/rotators endpoints read and
// write. pkg/rotatorstore provides the Postgres implementation; declaring it as
// an interface here keeps the handler tests off a database.
type RotatorStore interface {
	GetOrDefault(ctx context.Context, platform string) (rot.Config, bool, error)
	Put(ctx context.Context, platform string, cfg rot.Config) error
	Delete(ctx context.Context, platform string) error
}

// RotatorPublisher pushes saved copy to the platform's onscreens-server so the
// overlays pick it up without a restart. pkg/onscreens-client implements it.
type RotatorPublisher interface {
	PublishRotatorConfig(ctx context.Context, platform string, cfg rot.Config) error
}

// SetRotators installs the rotator store + publisher the console's
// /api/rotators endpoints use. Without it the endpoints report 503 rather than
// panicking, matching how the flag endpoints degrade before their client loads.
func (s *Server) SetRotators(store RotatorStore, pub RotatorPublisher) {
	s.rotators = store
	s.rotatorPub = pub
}

// rotatorConfigDTO is the JSON shape the console renders and submits.
//
// Budgets rides along so the editor can warn about copy that won't fit without
// hardcoding the corner widths — it measures candidate text in the overlay's own
// font (FontFamilyCSS) against FitWidthPx at MinFontSizePx.
//
// Defaults is the copy compiled into onscreens-server for this platform, sent so
// the editor can mark each line as shipped-with-the-product or authored here.
// Provenance is derived by comparing against this rather than recorded on the
// message, deliberately: it's a property of "how does this line compare to the
// binary", not of the line itself, so it has no business in the stored document
// or in what goes to onscreens-server.
//
// Stored reports whether Config is saved copy or a prefill of those defaults.
type rotatorConfigDTO struct {
	Platform string       `json:"platform"`
	Stored   bool         `json:"stored"`
	Config   rot.Config   `json:"config"`
	Defaults rot.Config   `json:"defaults"`
	Budgets  []rot.Budget `json:"budgets"`
}

// newRotatorConfigDTO builds the response every rotator endpoint returns, so the
// three of them can't drift on which context the editor gets.
func newRotatorConfigDTO(platform string, stored bool, cfg rot.Config) rotatorConfigDTO {
	return rotatorConfigDTO{
		Platform: platform,
		Stored:   stored,
		Config:   cfg,
		Defaults: rot.DefaultConfigFor(platform),
		Budgets:  rot.Budgets(),
	}
}

// knownPlatform guards the platform path segment. The platform is a primary key
// and a NATS subject leaf, so an arbitrary string would let a typo write a row
// no onscreens-server ever reads.
func knownPlatform(platform string) bool {
	switch platform {
	case rot.PlatformTwitch, rot.PlatformYouTube, rot.PlatformTikTok,
		rot.PlatformInstagram, rot.PlatformFacebook:
		return true
	default:
		return false
	}
}

// rotatorsReady reports whether the store + publisher are wired, writing the
// 503 when they aren't.
func (s *Server) rotatorsReady(w http.ResponseWriter) bool {
	if s.rotators == nil || s.rotatorPub == nil {
		http.Error(w, "rotator editing unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// rotatorsGetHandler serves one platform's rotator copy — the stored document,
// or the compiled-in defaults filtered to that platform when it's never been
// edited. GET /api/rotators/{platform}.
func (s *Server) rotatorsGetHandler(w http.ResponseWriter, r *http.Request) {
	if !s.rotatorsReady(w) {
		return
	}
	platform := mux.Vars(r)["platform"]
	if !knownPlatform(platform) {
		http.Error(w, "unknown platform", http.StatusNotFound)
		return
	}
	cfg, stored, err := s.rotators.GetOrDefault(r.Context(), platform)
	if err != nil {
		// GetOrDefault still returns usable defaults on error, so serve them and
		// log rather than failing the console's page load.
		slog.WarnContext(r.Context(), "rotator config read failed; serving defaults",
			"err", err, "platform", platform)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(newRotatorConfigDTO(platform, stored, cfg))
}

// rotatorsPutHandler saves one platform's rotator copy and pushes it live.
// PUT /api/rotators/{platform}.
//
// Validation failures come back as 422 with the message naming the offending
// line, so the console can point at it instead of just refusing the save. The
// DB write happens before the publish: a failed publish leaves the copy stored
// (onscreens-server picks it up on its next restart or republish), whereas
// publishing first could put copy on screen that no restart would restore.
func (s *Server) rotatorsPutHandler(w http.ResponseWriter, r *http.Request) {
	if !s.rotatorsReady(w) {
		return
	}
	platform := mux.Vars(r)["platform"]
	if !knownPlatform(platform) {
		http.Error(w, "unknown platform", http.StatusNotFound)
		return
	}
	var body rot.Config
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	cfg, err := rot.Sanitize(body)
	if err != nil {
		var verr *rot.ValidationError
		if errors.As(err, &verr) {
			http.Error(w, verr.Error(), http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := s.rotators.Put(r.Context(), platform, cfg); err != nil {
		slog.ErrorContext(r.Context(), "rotator config save failed", "err", err, "platform", platform)
		http.Error(w, "couldn't save rotator config", http.StatusInternalServerError)
		return
	}
	if err := s.rotatorPub.PublishRotatorConfig(r.Context(), platform, cfg); err != nil {
		slog.ErrorContext(r.Context(), "rotator config publish failed", "err", err, "platform", platform)
	}
	slog.InfoContext(r.Context(), "rotator config saved via console",
		"platform", platform,
		"left_messages", len(cfg.Left.Messages), "right_messages", len(cfg.Right.Messages),
		"left_promo", len(cfg.Left.PromoMessages), "right_promo", len(cfg.Right.PromoMessages))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(newRotatorConfigDTO(platform, true, cfg))
}

// rotatorsResetHandler drops a platform's stored copy, reverting it to the
// defaults compiled into onscreens-server, and pushes those defaults live so the
// overlays don't wait for a restart. DELETE /api/rotators/{platform}.
func (s *Server) rotatorsResetHandler(w http.ResponseWriter, r *http.Request) {
	if !s.rotatorsReady(w) {
		return
	}
	platform := mux.Vars(r)["platform"]
	if !knownPlatform(platform) {
		http.Error(w, "unknown platform", http.StatusNotFound)
		return
	}
	if err := s.rotators.Delete(r.Context(), platform); err != nil {
		slog.ErrorContext(r.Context(), "rotator config reset failed", "err", err, "platform", platform)
		http.Error(w, "couldn't reset rotator config", http.StatusInternalServerError)
		return
	}
	cfg := rot.DefaultConfigFor(platform)
	if err := s.rotatorPub.PublishRotatorConfig(r.Context(), platform, cfg); err != nil {
		slog.ErrorContext(r.Context(), "rotator config publish failed", "err", err, "platform", platform)
	}
	slog.InfoContext(r.Context(), "rotator config reset to defaults via console", "platform", platform)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(newRotatorConfigDTO(platform, false, cfg))
}
