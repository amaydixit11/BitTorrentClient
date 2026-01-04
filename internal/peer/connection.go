package peer

import (
	"context"
	"fmt"
	"io"
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

// Connection represents a connection to a peer with download capabilities
type Connection struct {
	*Peer
	requestQueue chan *RequestItem
	pieceQueue   chan *PieceData
	done         chan struct{}
	mu           sync.RWMutex // Add mutex for thread safety
	connected    bool         // Track connection state
	stopOnce     sync.Once    // Ensure Stop() is only called once
	stopped      bool         // Track if connection is stopped
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
		connected:    true,
	}
}

func (c *Connection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && !c.stopped
}

// Stop stops the connection - thread-safe, can be called multiple times
// Replace this function in internal/peer/connection.go
func (c *Connection) Stop() {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		c.stopped = true
		c.connected = false
		c.mu.Unlock()

		close(c.done)

		// CRITICAL FIX: Close the pieceQueue channel to signal downstream listeners
		// that no more data will be sent.
		close(c.pieceQueue)

		if c.Conn != nil {
			c.Conn.Close()
		}
	})
}

// IsStopped returns true if the connection has been stopped
func (c *Connection) IsStopped() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stopped
}

// RequestPiece queues a piece request
func (c *Connection) RequestPiece(pieceIndex, begin int64, length int64) error {
	if c.IsStopped() {
		return fmt.Errorf("connection stopped")
	}

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

// handleMessage processes incoming messages
func (c *Connection) handleMessage(msg *Message) error {
	if msg == nil {
		// Keep-alive message - reset any timeout counters if needed
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch msg.ID {
	case MsgChoke:
		c.Choked = true
		fmt.Printf("Peer %x choked us\n", c.ID[:8])
		// Clear any pending requests since we're now choked
		c.clearPendingRequests()

	case MsgUnchoke:
		c.Choked = false
		fmt.Printf("Peer %x unchoked us\n", c.ID[:8])
		// We can now start requesting pieces again

	case MsgInterested:
		c.Interested = true
		fmt.Printf("Peer %x is interested in our pieces\n", c.ID[:8])

	case MsgNotInterested:
		c.Interested = false
		fmt.Printf("Peer %x is not interested in our pieces\n", c.ID[:8])

	case MsgHave:
		if len(msg.Payload) != 4 {
			return fmt.Errorf("invalid have message payload length: %d", len(msg.Payload))
		}

		pieceIndex, err := ParseHaveMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid have message: %w", err)
		}

		// Validate piece index
		if pieceIndex < 0 {
			return fmt.Errorf("invalid piece index: %d", pieceIndex)
		}

		c.SetPiece(int(pieceIndex))
		fmt.Printf("Peer %x has piece %d\n", c.ID[:8], pieceIndex)

	case MsgBitfield:
		// Validate bitfield length
		if len(msg.Payload) == 0 {
			return fmt.Errorf("empty bitfield message")
		}

		// Initialize or update bitfield
		c.Bitfield = make([]byte, len(msg.Payload))
		copy(c.Bitfield, msg.Payload)
		fmt.Printf("Peer %x sent bitfield of length %d\n", c.ID[:8], len(msg.Payload))

	case MsgPiece:
		// Validate minimum payload length (4 bytes index + 4 bytes begin + at least 1 byte data)
		if len(msg.Payload) < 9 {
			return fmt.Errorf("invalid piece message payload length: %d", len(msg.Payload))
		}

		// Handle incoming piece data
		index, begin, data, err := ParsePieceMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid piece message: %w", err)
		}

		// Validate piece data
		if index < 0 || begin < 0 || len(data) == 0 {
			return fmt.Errorf("invalid piece data: index=%d, begin=%d, data_len=%d",
				index, begin, len(data))
		}

		fmt.Printf("Received piece %d, begin %d, length %d from peer %x\n",
			index, begin, len(data), c.ID[:8])

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
			// Queue full - this indicates a flow control issue
			fmt.Printf("Warning: piece queue full, dropping data for piece %d\n", index)
		}

	case MsgRequest:
		// Handle incoming request from peer (they want a piece from us)
		if len(msg.Payload) != 12 {
			return fmt.Errorf("invalid request message payload length: %d", len(msg.Payload))
		}

		index, begin, length, err := ParseRequestMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid request message: %w", err)
		}

		// Validate request parameters
		if index < 0 || begin < 0 || length <= 0 {
			return fmt.Errorf("invalid request parameters: index=%d, begin=%d, length=%d",
				index, begin, length)
		}

		// Check if we're choking this peer
		if c.Choking {
			fmt.Printf("Ignoring request from choked peer %x\n", c.ID[:8])
			return nil
		}

		// Check if we have the requested piece
		if !c.HasPiece(int(index)) {
			fmt.Printf("Peer %x requested piece %d that we don't have\n", c.ID[:8], index)
			return nil
		}

		fmt.Printf("Peer %x requested piece %d, begin %d, length %d\n",
			c.ID[:8], index, begin, length)

		// TODO: Implement piece serving logic
		// This would involve:
		// 1. Reading the piece data from disk
		// 2. Extracting the requested block
		// 3. Sending a piece message back to the peer
		// For now, we'll just log it
		fmt.Printf("TODO: Serve piece %d to peer %x\n", index, c.ID[:8])

	case MsgCancel:
		// Handle cancel request
		if len(msg.Payload) != 12 {
			return fmt.Errorf("invalid cancel message payload length: %d", len(msg.Payload))
		}

		index, begin, length, err := ParseCancelMessage(msg.Payload)
		if err != nil {
			return fmt.Errorf("invalid cancel message: %w", err)
		}

		fmt.Printf("Peer %x cancelled request for piece %d, begin %d, length %d\n",
			c.ID[:8], index, begin, length)

		// TODO: Cancel any pending uploads for this request
		// This would involve:
		// 1. Finding the matching request in our upload queue
		// 2. Removing it from the queue
		// 3. Freeing any associated resources
		// For now, we'll just log it
		fmt.Printf("TODO: Cancel upload for piece %d to peer %x\n", index, c.ID[:8])

	case MsgPort:
		// Handle port message (for DHT)
		if len(msg.Payload) != 2 {
			return fmt.Errorf("invalid port message payload length: %d", len(msg.Payload))
		}

		port := ParsePortMessage(msg.Payload)
		fmt.Printf("Peer %x DHT port: %d\n", c.ID[:8], port)

		// TODO: Store DHT port information if implementing DHT support

	// In your message handling switch statement, add:
	default:
		fmt.Printf("Unknown message ID %d from peer %s, payload length: %d\n",
			msg.ID, c.ID[:8], len(msg.Payload))
		// Don't return error - just continue processing

	}

	return nil
}

// clearPendingRequests clears any pending requests when we get choked
func (c *Connection) clearPendingRequests() {
	// Drain the request queue
	for {
		select {
		case <-c.requestQueue:
			// Request cleared
		default:
			return
		}
	}
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

// Add this helper struct at the top of the file
type readResult struct {
	msg *Message
	err error
}

// Replace the existing Start function with this one
func (c *Connection) Start() {
	// We'll use two goroutines: one for reading, one for the main logic.
	msgChan := make(chan readResult)
	go c.readLoop(msgChan)
	go c.messageLoop(msgChan)
}

// Add this new function to the file
// readLoop handles blocking reads from the peer connection and sends results on a channel.
func (c *Connection) readLoop(msgChan chan<- readResult) {
	for {
		// This is now a blocking read with no aggressive timeout.
		// It will wait as long as needed for a full message to arrive.
		msg, err := c.ReadMessage()

		// Send the result back to the main loop.
		select {
		case msgChan <- readResult{msg, err}:
			// If there was an error, we can't read anymore, so stop this goroutine.
			if err != nil {
				return
			}
		case <-c.done:
			// The connection is stopping, so exit the goroutine.
			return
		}
	}
}

// Replace the existing messageLoop function with this one
func (c *Connection) messageLoop(msgChan <-chan readResult) {
	defer func() {
		if !c.IsStopped() {
			c.Stop()
		}
	}()

	keepAliveTicker := time.NewTicker(2 * time.Minute)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-c.done:
			return

		case result := <-msgChan: // Wait for a message from the readLoop
			if result.err != nil {
				// A read error means the connection is dead.
				// io.EOF is a normal disconnect, no need to print an error for it.
				if result.err != io.EOF {
					fmt.Printf("ERROR: Corrupt message from peer %x: %v\n", c.ID[:8], result.err)
				}
				return // Stop this connection's logic loop.
			}

			// We got a message, so handle it.
			if err := c.handleMessage(result.msg); err != nil {
				fmt.Printf("ERROR: Failed to handle message from peer %x: %v\n", c.ID[:8], err)
				return // Stop if handling fails.
			}

		case req := <-c.requestQueue:
			if c.IsStopped() {
				return
			}
			err := c.SendMessage(NewRequestMessage(
				uint32(req.PieceIndex),
				uint32(req.Begin),
				uint32(req.Length),
			))
			if err != nil {
				fmt.Printf("ERROR: Failed to send request to peer %x: %v\n", c.ID[:8], err)
				return
			}

		case <-keepAliveTicker.C:
			if c.IsStopped() {
				return
			}
			if err := c.SendKeepAlive(); err != nil {
				fmt.Printf("ERROR: Failed to send keep-alive to peer %x: %v\n", c.ID[:8], err)
				return
			}
		}
	}
}
