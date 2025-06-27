package peer

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Message IDs
const (
	MsgChoke         = 0
	MsgUnchoke       = 1
	MsgInterested    = 2
	MsgNotInterested = 3
	MsgHave          = 4
	MsgBitfield      = 5
	MsgRequest       = 6
	MsgPiece         = 7
	MsgCancel        = 8
	MsgPort          = 9
)

// Message represents a peer wire protocol message
type Message struct {
	ID      byte
	Payload []byte
}

// NewMessage creates a new message
func NewMessage(id byte, payload []byte) *Message {
	return &Message{
		ID:      id,
		Payload: payload,
	}
}

// Serialize converts message to bytes
func (m *Message) Serialize() []byte {
	if m == nil {
		// Keep-alive message
		return make([]byte, 4)
	}

	length := 1 + len(m.Payload)
	buf := make([]byte, 4+length)

	// Length prefix
	binary.BigEndian.PutUint32(buf[0:4], uint32(length))

	// Message ID
	buf[4] = m.ID

	// Payload
	copy(buf[5:], m.Payload)

	return buf
}

// DeserializeMessage reads and parses a message from reader
func DeserializeMessage(r io.Reader) (*Message, error) {
	// Read length prefix
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	// Keep-alive message
	if length == 0 {
		return nil, nil
	}

	// Read message ID
	msgBuf := make([]byte, length)
	_, err = io.ReadFull(r, msgBuf)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:      msgBuf[0],
		Payload: msgBuf[1:],
	}, nil
}

// Helper functions for creating specific message types

// NewChokeMessage creates a choke message
func NewChokeMessage() *Message {
	return NewMessage(MsgChoke, nil)
}

// NewUnchokeMessage creates an unchoke message
func NewUnchokeMessage() *Message {
	return NewMessage(MsgUnchoke, nil)
}

// NewInterestedMessage creates an interested message
func NewInterestedMessage() *Message {
	return NewMessage(MsgInterested, nil)
}

// NewNotInterestedMessage creates a not interested message
func NewNotInterestedMessage() *Message {
	return NewMessage(MsgNotInterested, nil)
}

// NewHaveMessage creates a have message
func NewHaveMessage(pieceIndex uint32) *Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, pieceIndex)
	return NewMessage(MsgHave, payload)
}

// NewBitfieldMessage creates a bitfield message
func NewBitfieldMessage(bitfield []byte) *Message {
	return NewMessage(MsgBitfield, bitfield)
}

// NewCancelMessage creates a cancel message
func NewCancelMessage(index, begin, length uint32) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)
	return NewMessage(MsgCancel, payload)
}

func NewRequestMessage(index, begin, length uint32) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)

	return &Message{
		ID:      MsgRequest,
		Payload: payload,
	}
}

// ParseRequestMessage parses a request message payload
func ParseRequestMessage(payload []byte) (index, begin, length uint32, err error) {
	if len(payload) != 12 {
		return 0, 0, 0, fmt.Errorf("invalid request payload length: %d", len(payload))
	}

	index = binary.BigEndian.Uint32(payload[0:4])
	begin = binary.BigEndian.Uint32(payload[4:8])
	length = binary.BigEndian.Uint32(payload[8:12])

	return index, begin, length, nil
}

// ParsePieceMessage parses a piece message payload
func ParsePieceMessage(payload []byte) (index, begin uint32, data []byte, err error) {
	if len(payload) < 8 {
		return 0, 0, nil, fmt.Errorf("invalid piece payload length: %d", len(payload))
	}

	index = binary.BigEndian.Uint32(payload[0:4])
	begin = binary.BigEndian.Uint32(payload[4:8])
	data = payload[8:]

	return index, begin, data, nil
}

// NewPieceMessage creates a piece message
func NewPieceMessage(index, begin uint32, data []byte) *Message {
	payload := make([]byte, 8+len(data))
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	copy(payload[8:], data)

	return &Message{
		ID:      MsgPiece,
		Payload: payload,
	}
}

// ParseHaveMessage parses a have message payload
func ParseHaveMessage(payload []byte) (uint32, error) {
	if len(payload) != 4 {
		return 0, fmt.Errorf("invalid have payload length: %d", len(payload))
	}
	return binary.BigEndian.Uint32(payload), nil
}
