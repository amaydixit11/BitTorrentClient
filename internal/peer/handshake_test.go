package peer

import (
	"bytes"
	"testing"
)

func testIDs() (infoHash, peerID [20]byte) {
	for i := 0; i < 20; i++ {
		infoHash[i] = byte(i)
		peerID[i] = byte(200 - i)
	}
	return
}

// BEP-3 fixes the handshake at 68 bytes: 1 + 19 + 8 + 20 + 20.
func TestHandshakeSerializeLength(t *testing.T) {
	infoHash, peerID := testIDs()
	buf := NewHandshake(infoHash, peerID).Serialize()
	if len(buf) != 68 {
		t.Fatalf("handshake is %d bytes, want 68", len(buf))
	}
	if HandshakeSize != 68 {
		t.Errorf("HandshakeSize = %d, want 68", HandshakeSize)
	}
}

func TestHandshakeSerializeLayout(t *testing.T) {
	infoHash, peerID := testIDs()
	buf := NewHandshake(infoHash, peerID).Serialize()

	if buf[0] != 19 {
		t.Errorf("pstrlen = %d, want 19", buf[0])
	}
	if got := string(buf[1:20]); got != ProtocolString {
		t.Errorf("pstr = %q, want %q", got, ProtocolString)
	}
	// Reserved bytes must be zero when no extensions are negotiated.
	for i, b := range buf[20:28] {
		if b != 0 {
			t.Errorf("reserved byte %d = %d, want 0", i, b)
		}
	}
	if !bytes.Equal(buf[28:48], infoHash[:]) {
		t.Error("info hash not at bytes 28:48")
	}
	if !bytes.Equal(buf[48:68], peerID[:]) {
		t.Error("peer id not at bytes 48:68")
	}
}

func TestHandshakeRoundTrip(t *testing.T) {
	infoHash, peerID := testIDs()
	buf := NewHandshake(infoHash, peerID).Serialize()

	got, err := DeserializeHandshake(buf)
	if err != nil {
		t.Fatalf("DeserializeHandshake returned error: %v", err)
	}
	if got.Pstr != ProtocolString {
		t.Errorf("Pstr = %q, want %q", got.Pstr, ProtocolString)
	}
	if got.InfoHash != infoHash {
		t.Errorf("InfoHash = %x, want %x", got.InfoHash, infoHash)
	}
	if got.PeerID != peerID {
		t.Errorf("PeerID = %x, want %x", got.PeerID, peerID)
	}
}

func TestDeserializeHandshakeTooShort(t *testing.T) {
	infoHash, peerID := testIDs()
	full := NewHandshake(infoHash, peerID).Serialize()
	for _, n := range []int{0, 1, 19, 67} {
		if _, err := DeserializeHandshake(full[:n]); err == nil {
			t.Errorf("DeserializeHandshake(%d bytes) = nil error, want error", n)
		}
	}
}

func TestDeserializeHandshakeRejectsWrongProtocol(t *testing.T) {
	infoHash, peerID := testIDs()
	buf := NewHandshake(infoHash, peerID).Serialize()

	bad := append([]byte(nil), buf...)
	bad[0] = 18 // wrong pstrlen
	if _, err := DeserializeHandshake(bad); err == nil {
		t.Error("wrong pstrlen accepted, want error")
	}

	bad = append([]byte(nil), buf...)
	copy(bad[1:20], "NotTorrent protocol")
	if _, err := DeserializeHandshake(bad); err == nil {
		t.Error("wrong protocol string accepted, want error")
	}
}

// A peer may set reserved bits (DHT, fast extension). We must still parse it.
func TestDeserializeHandshakeToleratesReservedBits(t *testing.T) {
	infoHash, peerID := testIDs()
	buf := NewHandshake(infoHash, peerID).Serialize()
	buf[27] |= 0x01 // DHT bit
	buf[25] |= 0x10

	got, err := DeserializeHandshake(buf)
	if err != nil {
		t.Fatalf("reserved bits rejected: %v", err)
	}
	if got.InfoHash != infoHash {
		t.Error("info hash mangled when reserved bits set")
	}
}
