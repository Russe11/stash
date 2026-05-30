package api

import (
	"testing"
	"time"
)

// TestNewAPIHTTPServerTimeouts pins the connection-phase timeout invariants: the header read is
// bounded (slow-loris guard) while body reads, response writes, and idle/streaming are deliberately
// UNBOUNDED — a regression that sets a blanket Read/WriteTimeout would silently break large uploads,
// range downloads, and long-lived subscriptions.
func TestNewAPIHTTPServerTimeouts(t *testing.T) {
	s := newAPIHTTPServer(":0", nil, nil)

	if s.ReadHeaderTimeout != readHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", s.ReadHeaderTimeout, readHeaderTimeout)
	}
	if readHeaderTimeout <= 0 {
		t.Errorf("readHeaderTimeout must be > 0 (slow-loris guard), got %v", readHeaderTimeout)
	}
	// Sanity bound so nobody sets it absurdly high and effectively disables the guard.
	if readHeaderTimeout > 2*time.Minute {
		t.Errorf("readHeaderTimeout = %v, want <= 2m", readHeaderTimeout)
	}

	if s.ReadTimeout != 0 {
		t.Errorf("ReadTimeout must stay 0 (unbounded body reads), got %v", s.ReadTimeout)
	}
	if s.WriteTimeout != 0 {
		t.Errorf("WriteTimeout must stay 0 (unbounded streaming/downloads), got %v", s.WriteTimeout)
	}
	if s.IdleTimeout != 0 {
		t.Errorf("IdleTimeout must stay 0 (unbounded keep-alive), got %v", s.IdleTimeout)
	}

	// http/2 is disabled (empty, non-nil TLSNextProto) so connections stay hijackable — required to
	// stop running streams when a scene file is deleted.
	if s.TLSNextProto == nil {
		t.Error("TLSNextProto must be a non-nil (empty) map to disable http/2")
	}
	if len(s.TLSNextProto) != 0 {
		t.Errorf("TLSNextProto must be empty, got %d entries", len(s.TLSNextProto))
	}
}

// TestNewGraphQLWebsocketUpgrader pins the WebSocket upgrade guard: a bounded handshake (the HTTP
// header timeout stops applying once the connection is hijacked) and a permissive origin check (auth
// is enforced by the connection-init payload, not the Origin header).
func TestNewGraphQLWebsocketUpgrader(t *testing.T) {
	u := newGraphQLWebsocketUpgrader()

	if u.HandshakeTimeout != wsHandshakeTimeout {
		t.Errorf("HandshakeTimeout = %v, want %v", u.HandshakeTimeout, wsHandshakeTimeout)
	}
	if wsHandshakeTimeout <= 0 {
		t.Errorf("wsHandshakeTimeout must be > 0 (half-open upgrade guard), got %v", wsHandshakeTimeout)
	}
	if wsHandshakeTimeout > time.Minute {
		t.Errorf("wsHandshakeTimeout = %v, want <= 1m", wsHandshakeTimeout)
	}

	if u.CheckOrigin == nil {
		t.Fatal("CheckOrigin must be set")
	}
	if !u.CheckOrigin(nil) {
		t.Error("CheckOrigin should accept all origins (auth is enforced post-upgrade)")
	}
}
