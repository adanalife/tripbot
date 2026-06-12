package vlcServer

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
)

// rtspProbeTimeout caps each DESCRIBE probe so a wedged listener can't hang
// the watchdog loop. Also bounds the /health/rtsp handler's worst case.
const rtspProbeTimeout = 3 * time.Second

// rtspProbeAddr is the local libvlc RTSP listener we probe. The path
// (`/dashcam`) is hardcoded to match the sout chain in vlc.go's
// mediaOptions; keeping them coupled in code keeps the probe meaningful.
const rtspProbeAddr = "localhost:8554"

// resumeMarkerName is the file under RunDir that captures the
// currently-playing video at self-heal time. main() consults it on startup
// to resume playback after a watchdog-triggered restart.
const resumeMarkerName = "vlc-server-resume-from.txt"

// ResumeMarkerPath returns the absolute path to the resume marker, scoped
// to the configured RunDir so it shares the same writable volume as the
// pidfile.
func ResumeMarkerPath() string {
	return filepath.Join(c.Conf.RunDir, resumeMarkerName)
}

// formatResumeMarker renders the marker file content: the basename on the
// first line, the playback position (ms) on the second. Two lines rather
// than one delimited line so the original basename-only format stays a valid
// prefix of the new one.
func formatResumeMarker(file string, positionMs int64) []byte {
	return []byte(file + "\n" + strconv.FormatInt(positionMs, 10) + "\n")
}

// ParseResumeMarker decodes marker file content. The position line is
// optional (markers written before positions existed have only the
// basename); a missing or malformed position decodes as 0 = start of clip.
func ParseResumeMarker(data []byte) (file string, positionMs int64) {
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	file = strings.TrimSpace(lines[0])
	if len(lines) > 1 {
		if ms, err := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64); err == nil && ms > 0 {
			positionMs = ms
		}
	}
	return file, positionMs
}

// probeRTSPDescribe sends a single RTSP DESCRIBE to the local listener and
// returns nil iff the server answers with a 200 status. Used by both
// /health/rtsp and the self-heal watchdog.
//
// The libvlc RTSP server can answer OPTIONS (process alive, port bound)
// while DESCRIBE returns 500 — the silent-failure mode that motivates this
// probe: the sout chain went away without the player crashing or releasing
// :8554.
func probeRTSPDescribe() error {
	return probeRTSPDescribeAt(rtspProbeAddr)
}

func probeRTSPDescribeAt(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, rtspProbeTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(rtspProbeTimeout))
	req := "DESCRIBE rtsp://" + addr + "/dashcam RTSP/1.0\r\n" +
		"CSeq: 1\r\n" +
		"Accept: application/sdp\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("write DESCRIBE: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read status: %w", err)
	}
	status := strings.TrimSpace(line)
	if !strings.HasPrefix(status, "RTSP/1.0 200") {
		return fmt.Errorf("unexpected status: %s", status)
	}
	return nil
}

// StartRTSPWatchdog launches a goroutine that probes the RTSP listener on
// `interval` and, after `failureThreshold` consecutive failures, persists a
// resume marker with the currently-playing filename and signals SIGTERM to
// the process so the supervisor (supervisord in the container) restarts it.
//
// `initialDelay` lets libvlc bring up the first Media's sout chain before
// probing starts — cold-start DESCRIBE returns 500 for a few seconds while
// the player is still loading, which is normal.
func (s *Server) StartRTSPWatchdog(ctx context.Context, interval time.Duration, failureThreshold int, initialDelay time.Duration) {
	go func() {
		slog.InfoContext(ctx, "starting RTSP watchdog",
			"interval", interval,
			"failure_threshold", failureThreshold,
			"initial_delay", initialDelay,
		)
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialDelay):
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		consecutive := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := probeRTSPDescribe(); err != nil {
					consecutive++
					slog.WarnContext(ctx, "RTSP DESCRIBE failed",
						"err", err,
						"consecutive", consecutive,
						"threshold", failureThreshold,
					)
					if consecutive >= failureThreshold {
						s.triggerSelfHeal(ctx)
						return
					}
					continue
				}
				if consecutive > 0 {
					slog.InfoContext(ctx, "RTSP DESCRIBE recovered", "after_failures", consecutive)
				}
				consecutive = 0
			}
		}
	}()
}

// triggerSelfHeal persists the resume marker and signals SIGTERM to the
// process. The signal flows into the existing gracefulShutdown handler in
// cmd/vlc-server, which releases libvlc + the :8554 socket before exit —
// without that, supervisord's respawn collides with TIME_WAIT and burns
// retries.
func (s *Server) triggerSelfHeal(ctx context.Context) {
	current := strings.TrimSpace(s.currentlyPlaying())
	marker := ResumeMarkerPath()
	if current == "" {
		slog.WarnContext(ctx, "RTSP self-heal: no current video; skipping marker write", "marker", marker)
	} else {
		// Best-effort position capture — the player is mid-failure, so a
		// read error just means the respawn resumes from the clip's start.
		var posMs int64
		if ms, err := s.Player.MediaTime(); err == nil {
			posMs = int64(ms)
		}
		slog.ErrorContext(ctx, "RTSP self-heal: persisting resume marker and signaling SIGTERM",
			"resume_from", current,
			"position_ms", posMs,
			"marker", marker,
		)
		if err := os.WriteFile(marker, formatResumeMarker(current, posMs), 0o644); err != nil {
			slog.ErrorContext(ctx, "failed to write resume marker", "err", err, "marker", marker)
		}
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		slog.ErrorContext(ctx, "failed to send SIGTERM to self; using os.Exit", "err", err)
		os.Exit(1)
	}
}
