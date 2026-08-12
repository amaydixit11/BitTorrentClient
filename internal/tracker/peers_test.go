package tracker

import (
	"testing"
)

// BEP-23 compact format: 6 bytes per peer, 4-byte IPv4 + 2-byte big-endian port.
func TestParseBinaryPeers(t *testing.T) {
	tc := &TrackerClient{}
	data := []byte{
		192, 168, 1, 1, 0x1A, 0xE1, // 192.168.1.1:6881
		10, 0, 0, 5, 0x1F, 0x90, // 10.0.0.5:8080
	}

	peers, err := tc.parsePeers(string(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("got %d peers, want 2", len(peers))
	}
	if got := peers[0].IP.String(); got != "192.168.1.1" {
		t.Errorf("peer 0 IP = %s, want 192.168.1.1", got)
	}
	if peers[0].Port != 6881 {
		t.Errorf("peer 0 port = %d, want 6881", peers[0].Port)
	}
	if got := peers[1].IP.String(); got != "10.0.0.5" {
		t.Errorf("peer 1 IP = %s, want 10.0.0.5", got)
	}
	if peers[1].Port != 8080 {
		t.Errorf("peer 1 port = %d, want 8080", peers[1].Port)
	}
}

func TestParseBinaryPeersEmpty(t *testing.T) {
	tc := &TrackerClient{}
	peers, err := tc.parsePeers("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("got %d peers, want 0", len(peers))
	}
}

// A truncated compact list must be rejected, not silently rounded down.
func TestParseBinaryPeersMisaligned(t *testing.T) {
	tc := &TrackerClient{}
	for _, n := range []int{1, 5, 7, 11} {
		if _, err := tc.parsePeers(string(make([]byte, n))); err == nil {
			t.Errorf("parsePeers(%d bytes) = nil error, want error", n)
		}
	}
}

func TestParseBinaryPeersHighPort(t *testing.T) {
	tc := &TrackerClient{}
	peers, err := tc.parsePeers(string([]byte{1, 2, 3, 4, 0xFF, 0xFF}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peers[0].Port != 65535 {
		t.Errorf("port = %d, want 65535", peers[0].Port)
	}
}

func TestParseDictPeers(t *testing.T) {
	tc := &TrackerClient{}
	data := []interface{}{
		map[string]interface{}{
			"ip":      "192.168.1.100",
			"port":    int64(6881),
			"peer id": "-AM0001-123456789012",
		},
		map[string]interface{}{
			"ip":   "10.1.2.3",
			"port": int64(51413),
		},
	}

	peers, err := tc.parsePeers(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("got %d peers, want 2", len(peers))
	}
	if got := peers[0].IP.String(); got != "192.168.1.100" {
		t.Errorf("peer 0 IP = %s", got)
	}
	if peers[0].Port != 6881 {
		t.Errorf("peer 0 port = %d, want 6881", peers[0].Port)
	}
	if string(peers[0].ID) != "-AM0001-123456789012" {
		t.Errorf("peer 0 id = %q", peers[0].ID)
	}
	if peers[1].ID != nil {
		t.Errorf("peer 1 id = %q, want nil when absent", peers[1].ID)
	}
}

func TestParseDictPeersRejectsMalformed(t *testing.T) {
	tc := &TrackerClient{}
	cases := []struct {
		name string
		in   []interface{}
	}{
		{"not a dict", []interface{}{"nope"}},
		{"missing ip", []interface{}{map[string]interface{}{"port": int64(1)}}},
		{"invalid ip", []interface{}{map[string]interface{}{"ip": "999.1.1.1", "port": int64(1)}}},
		{"missing port", []interface{}{map[string]interface{}{"ip": "1.1.1.1"}}},
		{"port wrong type", []interface{}{map[string]interface{}{"ip": "1.1.1.1", "port": "6881"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := tc.parsePeers(c.in); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

func TestParsePeersUnsupportedFormat(t *testing.T) {
	tc := &TrackerClient{}
	if _, err := tc.parsePeers(int64(42)); err == nil {
		t.Error("got nil error, want error for unsupported peers format")
	}
}
