package peer

import (
	"fmt"
	"net"
	"time"
)

// ConnectToPeer establishes a connection to a peer and performs handshake
func ConnectToPeer(address string, infoHash, peerID [20]byte) (*Peer, error) {
	// Establish TCP connection
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to peer %s: %w", address, err)
	}

	// Perform handshake
	handshake, err := PerformHandshake(conn, infoHash, peerID)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed with peer %s: %w", address, err)
	}

	// Create peer instance
	peer := NewPeer(conn, infoHash)
	peer.ID = handshake.PeerID

	return peer, nil
}

// SendMessage sends a message to the peer
func (p *Peer) SendMessage(msg *Message) error {
	data := msg.Serialize()
	_, err := p.Conn.Write(data)
	return err
}

// ReadMessage reads a message from the peer
func (p *Peer) ReadMessage() (*Message, error) {
	return DeserializeMessage(p.Conn)
}

// SendKeepAlive sends a keep-alive message
func (p *Peer) SendKeepAlive() error {
	return p.SendMessage(nil)
}

// SendChoke sends a choke message
func (p *Peer) SendChoke() error {
	p.Choking = true
	return p.SendMessage(NewChokeMessage())
}

// SendUnchoke sends an unchoke message
func (p *Peer) SendUnchoke() error {
	p.Choking = false
	return p.SendMessage(NewUnchokeMessage())
}

// SendInterested sends an interested message
func (p *Peer) SendInterested() error {
	p.Interesting = true
	return p.SendMessage(NewInterestedMessage())
}

// SendNotInterested sends a not interested message
func (p *Peer) SendNotInterested() error {
	p.Interesting = false
	return p.SendMessage(NewNotInterestedMessage())
}

// SendHave sends a have message
func (p *Peer) SendHave(pieceIndex uint32) error {
	return p.SendMessage(NewHaveMessage(pieceIndex))
}

// SendBitfield sends a bitfield message
func (p *Peer) SendBitfield(bitfield []byte) error {
	return p.SendMessage(NewBitfieldMessage(bitfield))
}

// SendRequest sends a request message
func (p *Peer) SendRequest(index, begin, length uint32) error {
	if p.Choked {
		return fmt.Errorf("peer is choking us")
	}
	return p.SendMessage(NewRequestMessage(index, begin, length))
}

// HandleMessage processes an incoming message and updates peer state
func (p *Peer) HandleMessage(msg *Message) error {
	if msg == nil {
		// Keep-alive message, do nothing
		return nil
	}

	switch msg.ID {
	case MsgChoke:
		p.Choked = true

	case MsgUnchoke:
		p.Choked = false

	case MsgInterested:
		p.Interested = true

	case MsgNotInterested:
		p.Interested = false

	case MsgHave:
		pieceIndex, err := ParseHaveMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid have message: %w", err)
		}
		p.SetPiece(int(pieceIndex))

	case MsgBitfield:
		// Initialize or update bitfield
		p.Bitfield = make([]byte, len(msg.Payload))
		copy(p.Bitfield, msg.Payload)

	case MsgRequest:
		// TODO: Handle incoming request (for serving files)

	case MsgPiece:
		// TODO: Handle incoming piece data

	case MsgCancel:
		// TODO: Handle cancel request

	default:
		return fmt.Errorf("unknown message ID: %d", msg.ID)
	}

	return nil
}
