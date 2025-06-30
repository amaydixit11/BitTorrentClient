package peer

import (
	"crypto/rand"
	"fmt"
	"sync"
)

// Client manages multiple peer connections
type Client struct {
	InfoHash [20]byte
	PeerID   [20]byte
	peers    map[string]*Peer
	mutex    sync.RWMutex
}

// NewClient creates a new peer client
func NewClient(infoHash [20]byte) *Client {
	var peerID [20]byte
	rand.Read(peerID[:])

	return &Client{
		InfoHash: infoHash,
		PeerID:   peerID,
		peers:    make(map[string]*Peer),
	}
}

// GetPeer returns a peer by address
func (c *Client) GetPeer(address string) (*Peer, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	peer, exists := c.peers[address]
	return peer, exists
}

// GetAllPeers returns all connected peers
func (c *Client) GetAllPeers() []*Peer {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	peers := make([]*Peer, 0, len(c.peers))
	for _, peer := range c.peers {
		peers = append(peers, peer)
	}

	return peers
}

// DisconnectPeer disconnects from a specific peer
func (c *Client) DisconnectPeer(address string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	peer, exists := c.peers[address]
	if !exists {
		return fmt.Errorf("peer %s not found", address)
	}

	err := peer.Close()
	delete(c.peers, address)

	return err
}

// DisconnectAll disconnects from all peers
func (c *Client) DisconnectAll() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var lastErr error
	for addr, peer := range c.peers {
		if err := peer.Close(); err != nil {
			lastErr = err
		}
		delete(c.peers, addr)
	}

	return lastErr
}
