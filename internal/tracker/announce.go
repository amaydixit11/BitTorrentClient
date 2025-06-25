package tracker

import (
	"bittorrentclient/internal/bencode"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// urlEncodeBytes properly URL encodes binary data for tracker requests
func urlEncodeBytes(data []byte) string {
	encoded := make([]byte, 0, len(data)*3)

	for _, b := range data {
		// Check if byte needs encoding
		if (b >= '0' && b <= '9') ||
			(b >= 'A' && b <= 'Z') ||
			(b >= 'a' && b <= 'z') ||
			b == '.' || b == '-' || b == '_' || b == '~' {
			encoded = append(encoded, b)
		} else {
			encoded = append(encoded, '%')
			encoded = append(encoded, "0123456789ABCDEF"[b>>4])
			encoded = append(encoded, "0123456789ABCDEF"[b&15])
		}
	}

	return string(encoded)
}

// buildTrackerURL constructs the tracker request URL with parameters
func (tc *TrackerClient) buildTrackerURL(announceURL string, req *TrackerRequest) (string, error) {
	u, err := url.Parse(announceURL)
	if err != nil {
		return "", fmt.Errorf("invalid announce URL: %v", err)
	}

	params := url.Values{}

	// Required parameters
	params.Set("info_hash", urlEncodeBytes(req.InfoHash))
	params.Set("peer_id", urlEncodeBytes(req.PeerID))
	params.Set("port", strconv.Itoa(req.Port))
	params.Set("uploaded", strconv.FormatInt(req.Uploaded, 10))
	params.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	params.Set("left", strconv.FormatInt(req.Left, 10))

	// Optional parameters
	if req.Compact {
		params.Set("compact", "1")
	}

	if req.NoPeerID {
		params.Set("no_peer_id", "1")
	}

	if req.Event != "" {
		params.Set("event", req.Event)
	}

	if req.IP != "" {
		params.Set("ip", req.IP)
	}

	if req.NumWant > 0 {
		params.Set("numwant", strconv.Itoa(req.NumWant))
	}

	if req.Key != "" {
		params.Set("key", req.Key)
	}

	if req.TrackerID != "" {
		params.Set("trackerid", req.TrackerID)
	}

	// Manually construct URL to avoid double-encoding
	baseURL := u.String()
	if u.RawQuery != "" {
		baseURL += "&" + params.Encode()
	} else {
		baseURL += "?" + params.Encode()
	}

	return baseURL, nil
}

// Announce sends an announce request to the tracker
func (tc *TrackerClient) Announce(announceURL string, req *TrackerRequest) (*TrackerResponse, error) {
	// Set default peer ID if not provided
	if req.PeerID == nil {
		req.PeerID = tc.peerID
	}

	// Set default port if not provided
	if req.Port == 0 {
		req.Port = tc.port
	}

	// Set default numwant if not provided
	if req.NumWant == 0 {
		req.NumWant = 50
	}

	// Build the request URL
	reqURL, err := tc.buildTrackerURL(announceURL, req)
	if err != nil {
		return nil, fmt.Errorf("failed to build tracker URL: %v", err)
	}

	// Make the HTTP request
	resp, err := tc.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tracker response: %v", err)
	}

	// Parse bencode response
	decoded, err := bencode.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %v", err)
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tracker response is not a dictionary")
	}

	// Parse the response
	return tc.parseTrackerResponse(dict)
}
