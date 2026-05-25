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
