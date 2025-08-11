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

// DELETE the urlEncodeBytes function.

// Replace this function in internal/tracker/announce.go
func (tc *TrackerClient) buildTrackerURL(announceURL string, req *TrackerRequest) (string, error) {
	u, err := url.Parse(announceURL)
	if err != nil {
		return "", fmt.Errorf("invalid announce URL: %v", err)
	}

	// Use url.Values for safe and idiomatic query parameter construction.
	q := u.Query()
	q.Set("info_hash", string(req.InfoHash)) // QueryEscape will handle the binary data correctly
	q.Set("peer_id", string(req.PeerID))
	q.Set("port", strconv.Itoa(req.Port))
	q.Set("uploaded", strconv.FormatInt(req.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	q.Set("left", strconv.FormatInt(req.Left, 10))

	if req.Compact {
		q.Set("compact", "1")
	}
	if req.Event != "" {
		q.Set("event", req.Event)
	}
	if req.NumWant > 0 {
		q.Set("numwant", strconv.Itoa(req.NumWant))
	}
	if req.TrackerID != "" {
		q.Set("trackerid", req.TrackerID)
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Announce sends an announce request to the tracker
func (tc *TrackerClient) Announce(announceURL string, req *TrackerRequest) (*TrackerResponse, error) {

	if req.PeerID == nil {
		req.PeerID = tc.peerID
	}

	if req.Port == 0 {
		req.Port = tc.port
	}

	if req.NumWant == 0 {
		req.NumWant = 50
	}

	reqURL, err := tc.buildTrackerURL(announceURL, req)
	if err != nil {
		return nil, fmt.Errorf("failed to build tracker URL: %v", err)
	}

	resp, err := tc.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tracker response: %v", err)
	}

	decoded, err := bencode.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %v", err)
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tracker response is not a dictionary")
	}

	respParsed, err := tc.parseTrackerResponse(dict)
	if err != nil {
		return nil, err
	}

	return respParsed, nil
}
