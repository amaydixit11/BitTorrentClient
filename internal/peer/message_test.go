package peer

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestMessageSerializeLayout(t *testing.T) {
	m := NewMessage(MsgHave, []byte{0x00, 0x00, 0x01, 0x02})
	buf := m.Serialize()

	if len(buf) != 4+1+4 {
		t.Fatalf("serialized length = %d, want 9", len(buf))
	}
	if got := binary.BigEndian.Uint32(buf[0:4]); got != 5 {
		t.Errorf("length prefix = %d, want 5 (id + payload)", got)
	}
	if buf[4] != MsgHave {
		t.Errorf("message id = %d, want %d", buf[4], MsgHave)
	}
	if !bytes.Equal(buf[5:], []byte{0x00, 0x00, 0x01, 0x02}) {
		t.Errorf("payload = %v", buf[5:])
	}
}

func TestSerializePayloadlessMessages(t *testing.T) {
	cases := []struct {
		name string
		msg  *Message
		id   byte
	}{
		{"choke", NewChokeMessage(), MsgChoke},
		{"unchoke", NewUnchokeMessage(), MsgUnchoke},
		{"interested", NewInterestedMessage(), MsgInterested},
		{"not interested", NewNotInterestedMessage(), MsgNotInterested},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := c.msg.Serialize()
			if len(buf) != 5 {
				t.Fatalf("length = %d, want 5", len(buf))
			}
			if got := binary.BigEndian.Uint32(buf[0:4]); got != 1 {
				t.Errorf("length prefix = %d, want 1", got)
			}
			if buf[4] != c.id {
				t.Errorf("id = %d, want %d", buf[4], c.id)
			}
		})
	}
}

// A nil *Message is the keep-alive: four zero bytes, no id.
func TestKeepAliveSerialize(t *testing.T) {
	var m *Message
	buf := m.Serialize()
	if !bytes.Equal(buf, []byte{0, 0, 0, 0}) {
		t.Errorf("keep-alive = %v, want four zero bytes", buf)
	}
}

func TestDeserializeKeepAlive(t *testing.T) {
	got, err := DeserializeMessage(bytes.NewReader([]byte{0, 0, 0, 0}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("keep-alive decoded as %#v, want nil", got)
	}
}

func TestMessageRoundTrip(t *testing.T) {
	cases := []*Message{
		NewChokeMessage(),
		NewUnchokeMessage(),
		NewInterestedMessage(),
		NewHaveMessage(1234),
		NewBitfieldMessage([]byte{0xFF, 0x0F, 0x00}),
		NewRequestMessage(1, 16384, 16384),
		NewMessage(MsgPiece, append([]byte{0, 0, 0, 3, 0, 0, 0, 0}, []byte("blockdata")...)),
	}
	for _, want := range cases {
		got, err := DeserializeMessage(bytes.NewReader(want.Serialize()))
		if err != nil {
			t.Fatalf("DeserializeMessage returned error: %v", err)
		}
		if got == nil {
			t.Fatal("DeserializeMessage returned nil for non-keep-alive")
		}
		if got.ID != want.ID {
			t.Errorf("id = %d, want %d", got.ID, want.ID)
		}
		if !bytes.Equal(got.Payload, want.Payload) {
			t.Errorf("payload = %v, want %v", got.Payload, want.Payload)
		}
	}
}

func TestDeserializeMessageTruncated(t *testing.T) {
	full := NewRequestMessage(1, 2, 3).Serialize()
	for _, n := range []int{0, 2, 3, len(full) - 1} {
		if _, err := DeserializeMessage(bytes.NewReader(full[:n])); err == nil {
			t.Errorf("DeserializeMessage(%d bytes) = nil error, want error", n)
		}
	}
}

func TestHaveMessageRoundTrip(t *testing.T) {
	for _, idx := range []uint32{0, 1, 255, 256, 65535, 4294967295} {
		m := NewHaveMessage(idx)
		if m.ID != MsgHave {
			t.Errorf("id = %d, want %d", m.ID, MsgHave)
		}
		got, err := ParseHaveMessage(m.Payload)
		if err != nil {
			t.Fatalf("ParseHaveMessage returned error: %v", err)
		}
		if got != idx {
			t.Errorf("ParseHaveMessage = %d, want %d", got, idx)
		}
	}
}

func TestParseHaveMessageInvalidLength(t *testing.T) {
	for _, payload := range [][]byte{{}, {1}, {1, 2, 3}, {1, 2, 3, 4, 5}} {
		if _, err := ParseHaveMessage(payload); err == nil {
			t.Errorf("ParseHaveMessage(%d bytes) = nil error, want error", len(payload))
		}
	}
}

func TestRequestMessageRoundTrip(t *testing.T) {
	const (
		wantIndex  = uint32(7)
		wantBegin  = uint32(32768)
		wantLength = uint32(16384)
	)
	m := NewRequestMessage(wantIndex, wantBegin, wantLength)
	if m.ID != MsgRequest {
		t.Errorf("id = %d, want %d", m.ID, MsgRequest)
	}
	if len(m.Payload) != 12 {
		t.Fatalf("payload = %d bytes, want 12", len(m.Payload))
	}

	index, begin, length, err := ParseRequestMessage(m.Payload)
	if err != nil {
		t.Fatalf("ParseRequestMessage returned error: %v", err)
	}
	if index != wantIndex || begin != wantBegin || length != wantLength {
		t.Errorf("got (%d,%d,%d), want (%d,%d,%d)",
			index, begin, length, wantIndex, wantBegin, wantLength)
	}
}

// Cancel shares the request wire format.
func TestParseCancelMatchesRequest(t *testing.T) {
	payload := NewRequestMessage(3, 16384, 16384).Payload
	i1, b1, l1, err1 := ParseRequestMessage(payload)
	i2, b2, l2, err2 := ParseCancelMessage(payload)
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v %v", err1, err2)
	}
	if i1 != i2 || b1 != b2 || l1 != l2 {
		t.Error("cancel and request parse differently")
	}
}

func TestParseRequestMessageInvalidLength(t *testing.T) {
	for _, n := range []int{0, 8, 11, 13} {
		if _, _, _, err := ParseRequestMessage(make([]byte, n)); err == nil {
			t.Errorf("ParseRequestMessage(%d bytes) = nil error, want error", n)
		}
	}
}

func TestParsePieceMessage(t *testing.T) {
	payload := []byte{
		0, 0, 0, 5, // index 5
		0, 0, 0x40, 0, // begin 16384
	}
	payload = append(payload, []byte("payload-bytes")...)

	index, begin, data, err := ParsePieceMessage(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if index != 5 {
		t.Errorf("index = %d, want 5", index)
	}
	if begin != 16384 {
		t.Errorf("begin = %d, want 16384", begin)
	}
	if string(data) != "payload-bytes" {
		t.Errorf("data = %q", data)
	}
}

func TestParsePieceMessageAllowsEmptyBlock(t *testing.T) {
	_, _, data, err := ParsePieceMessage([]byte{0, 0, 0, 1, 0, 0, 0, 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("data = %v, want empty", data)
	}
}

func TestParsePieceMessageTooShort(t *testing.T) {
	for _, n := range []int{0, 4, 7} {
		if _, _, _, err := ParsePieceMessage(make([]byte, n)); err == nil {
			t.Errorf("ParsePieceMessage(%d bytes) = nil error, want error", n)
		}
	}
}

func TestParsePortMessage(t *testing.T) {
	if got := ParsePortMessage([]byte{0x1A, 0xE1}); got != 6881 {
		t.Errorf("ParsePortMessage = %d, want 6881", got)
	}
	if got := ParsePortMessage([]byte{0x01}); got != 0 {
		t.Errorf("short port payload = %d, want 0", got)
	}
}

func TestMessageIDsMatchSpec(t *testing.T) {
	spec := map[string]byte{
		"choke": 0, "unchoke": 1, "interested": 2, "not interested": 3,
		"have": 4, "bitfield": 5, "request": 6, "piece": 7, "cancel": 8, "port": 9,
	}
	got := map[string]byte{
		"choke": MsgChoke, "unchoke": MsgUnchoke, "interested": MsgInterested,
		"not interested": MsgNotInterested, "have": MsgHave, "bitfield": MsgBitfield,
		"request": MsgRequest, "piece": MsgPiece, "cancel": MsgCancel, "port": MsgPort,
	}
	for name, want := range spec {
		if got[name] != want {
			t.Errorf("%s id = %d, want %d", name, got[name], want)
		}
	}
}
