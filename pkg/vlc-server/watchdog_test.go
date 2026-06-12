package vlcServer

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// startFakeRTSP listens on a free local port and answers a single request
// per connection with the supplied status line. Returns the addr and a
// cleanup func. Tests use it to feed probeRTSPDescribeAt known responses
// without needing a real libvlc.
func startFakeRTSP(t *testing.T, statusLine string) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				_, _ = bufio.NewReader(c).ReadString('\n')
				_, _ = c.Write([]byte(statusLine + "\r\nCSeq: 1\r\nContent-Length: 0\r\n\r\n"))
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestProbeRTSPDescribe_OK(t *testing.T) {
	addr, stop := startFakeRTSP(t, "RTSP/1.0 200 OK")
	defer stop()
	if err := probeRTSPDescribeAt(addr); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func TestProbeRTSPDescribe_500(t *testing.T) {
	addr, stop := startFakeRTSP(t, "RTSP/1.0 500 Internal server error")
	defer stop()
	err := probeRTSPDescribeAt(addr)
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error mentioning 500, got: %v", err)
	}
}

func TestProbeRTSPDescribe_NoListener(t *testing.T) {
	// 127.0.0.1:1 is a reserved port; nothing listens there.
	err := probeRTSPDescribeAt("127.0.0.1:1")
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Fatalf("expected dial error, got: %v", err)
	}
}

func TestResumeMarkerRoundTrip(t *testing.T) {
	file, pos := ParseResumeMarker(formatResumeMarker("wy_0042.MP4", 123_456))
	if file != "wy_0042.MP4" || pos != 123_456 {
		t.Fatalf("round-trip = %q@%d, want wy_0042.MP4@123456", file, pos)
	}
}

func TestParseResumeMarkerLegacyBasenameOnly(t *testing.T) {
	// Markers written before positions existed carry only the basename.
	file, pos := ParseResumeMarker([]byte("wy_0042.MP4\n"))
	if file != "wy_0042.MP4" || pos != 0 {
		t.Fatalf("legacy parse = %q@%d, want wy_0042.MP4@0", file, pos)
	}
}

func TestParseResumeMarkerMalformedPosition(t *testing.T) {
	file, pos := ParseResumeMarker([]byte("wy_0042.MP4\nnot-a-number\n"))
	if file != "wy_0042.MP4" || pos != 0 {
		t.Fatalf("malformed-position parse = %q@%d, want wy_0042.MP4@0", file, pos)
	}
	if f, p := ParseResumeMarker(nil); f != "" || p != 0 {
		t.Fatalf("empty parse = %q@%d, want \"\"@0", f, p)
	}
}
