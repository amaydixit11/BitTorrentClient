package peer

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// ConnectToPeer establishes a connection to a peer and performs handshake
func ConnectToPeer(ctx context.Context, address string, infoHash, peerID [20]byte) (*Peer, error) {
	// Use context-aware dialer
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
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
		// This will be implemented in later phases

	case MsgPiece:
		// Handle incoming piece data
		// This will be implemented in phase 4

	case MsgCancel:
		// Handle cancel request
		// This will be implemented in later phases if needed

	default:
		return fmt.Errorf("unknown message ID: %d", msg.ID)
	}

	return nil
}

// Connection represents a connection to a peer with download capabilities
type Connection struct {
	*Peer
	requestQueue chan *RequestItem
	pieceQueue   chan *PieceData
	done         chan struct{}
	mu           sync.RWMutex // Add mutex for thread safety
	connected    bool         // Track connection state
}

// RequestItem represents a piece request
type RequestItem struct {
	PieceIndex int64
	Begin      int64
	Length     int64
}

// PieceData represents received piece data
type PieceData struct {
	PieceIndex int64
	Begin      int64
	Data       []byte
}

// NewConnection creates a new peer connection
func NewConnection(conn net.Conn, infoHash [20]byte) *Connection {
	return &Connection{
		Peer:         NewPeer(conn, infoHash),
		requestQueue: make(chan *RequestItem, 100),
		pieceQueue:   make(chan *PieceData, 100),
		done:         make(chan struct{}),
	}
}

func (c *Connection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Start starts the connection message loop
func (c *Connection) Start() {
	go c.messageLoop()
}

// Stop stops the connection
func (c *Connection) Stop() {
	close(c.done)
	c.Close()
}

// RequestPiece queues a piece request
func (c *Connection) RequestPiece(pieceIndex, begin int64, length int64) error {
	if c.Choked {
		return fmt.Errorf("peer is choking us")
	}

	select {
	case c.requestQueue <- &RequestItem{
		PieceIndex: pieceIndex,
		Begin:      begin,
		Length:     length,
	}:
		return nil
	case <-c.done:
		return fmt.Errorf("connection closed")
	default:
		return fmt.Errorf("request queue full")
	}
}

// GetPieceData returns a channel for receiving piece data
func (c *Connection) GetPieceData() <-chan *PieceData {
	return c.pieceQueue
}

// messageLoop handles incoming and outgoing messages
func (c *Connection) messageLoop() {
	defer c.Stop()

	// Set up ticker for keep-alive messages
	keepAliveTicker := time.NewTicker(2 * time.Minute)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-c.done:
			return

		case req := <-c.requestQueue:
			// Send request message
			err := c.SendMessage(NewRequestMessage(
				uint32(req.PieceIndex),
				uint32(req.Begin),
				uint32(req.Length),
			))
			if err != nil {
				fmt.Printf("Failed to send request: %v\n", err)
				return
			}

		case <-keepAliveTicker.C:
			// Send keep-alive
			err := c.SendKeepAlive()
			if err != nil {
				fmt.Printf("Failed to send keep-alive: %v\n", err)
				return
			}

		default:
			// Read incoming messages
			c.Conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			msg, err := c.ReadMessage()
			if err != nil {
				// Check if it's a timeout
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				fmt.Printf("Failed to read message: %v\n", err)
				return
			}

			// Handle message
			err = c.handleMessage(msg)
			if err != nil {
				fmt.Printf("Failed to handle message: %v\n", err)
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Connection) handleMessage(msg *Message) error {
	if msg == nil {
		// Keep-alive message
		return nil
	}

	switch msg.ID {
	case MsgChoke:
		c.Choked = true
		fmt.Printf("Peer %x choked us\n", c.ID[:8])

	case MsgUnchoke:
		c.Choked = false
		fmt.Printf("Peer %x unchoked us\n", c.ID[:8])

	case MsgInterested:
		c.Interested = true

	case MsgNotInterested:
		c.Interested = false

	case MsgHave:
		pieceIndex, err := ParseHaveMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid have message: %w", err)
		}
		c.SetPiece(int(pieceIndex))

	case MsgBitfield:
		// Initialize or update bitfield
		c.Bitfield = make([]byte, len(msg.Payload))
		copy(c.Bitfield, msg.Payload)

	case MsgPiece:
		// Handle incoming piece data
		index, begin, data, err := ParsePieceMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid piece message: %w", err)
		}

		// Send piece data to piece queue
		select {
		case c.pieceQueue <- &PieceData{
			PieceIndex: int64(index),
			Begin:      int64(begin),
			Data:       data,
		}:
		case <-c.done:
			return fmt.Errorf("connection closed")
		default:
			// Queue full, drop the data (shouldn't happen with proper flow control)
			fmt.Printf("Warning: piece queue full, dropping data\n")
		}

	case MsgRequest:
		// TODO: Handle incoming request (for serving files)
		// This will be implemented in later phases

	case MsgCancel:
		// TODO: Handle cancel request
		// This will be implemented in later phases if needed

	default:
		return fmt.Errorf("unknown message ID: %d", msg.ID)
	}

	return nil
}

// IsUseful returns true if this peer has pieces we need
func (c *Connection) IsUseful(completedPieces map[int]bool, totalPieces int) bool {
	if c.Bitfield == nil {
		return false
	}

	for i := 0; i < totalPieces; i++ {
		if !completedPieces[i] && c.HasPiece(i) {
			return true
		}
	}
	return false
}
