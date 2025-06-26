package tracker

import (
	"bittorrentclient/internal/bencode"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

	// Build query string manually to avoid double encoding
	var parts []string

	// Required parameters
	parts = append(parts, "info_hash="+urlEncodeBytes(req.InfoHash))
	parts = append(parts, "peer_id="+urlEncodeBytes(req.PeerID))
	parts = append(parts, "port="+strconv.Itoa(req.Port))
	parts = append(parts, "uploaded="+strconv.FormatInt(req.Uploaded, 10))
	parts = append(parts, "downloaded="+strconv.FormatInt(req.Downloaded, 10))
	parts = append(parts, "left="+strconv.FormatInt(req.Left, 10))

	// Optional parameters
	if req.Compact {
		parts = append(parts, "compact=1")
	}

	if req.NoPeerID {
		parts = append(parts, "no_peer_id=1")
	}

	if req.Event != "" {
		parts = append(parts, "event="+url.QueryEscape(req.Event))
	}

	if req.IP != "" {
		parts = append(parts, "ip="+url.QueryEscape(req.IP))
	}

	if req.NumWant > 0 {
		parts = append(parts, "numwant="+strconv.Itoa(req.NumWant))
	}

	if req.Key != "" {
		parts = append(parts, "key="+url.QueryEscape(req.Key))
	}

	if req.TrackerID != "" {
		parts = append(parts, "trackerid="+url.QueryEscape(req.TrackerID))
	}

	queryString := strings.Join(parts, "&")

	if u.RawQuery != "" {
		return u.String() + "&" + queryString, nil
	}
	return u.String() + "?" + queryString, nil
}

// Announce sends an announce request to the tracker
func (tc *TrackerClient) Announce(announceURL string, req *TrackerRequest) (*TrackerResponse, error) {
	log.Println("Starting Announce")

	if req.PeerID == nil {
		log.Println("PeerID not provided in request, using default")
		req.PeerID = tc.peerID
	}

	if req.Port == 0 {
		log.Println("Port not provided in request, using default")
		req.Port = tc.port
	}

	if req.NumWant == 0 {
		log.Println("NumWant not provided in request, defaulting to 50")
		req.NumWant = 50
	}

	log.Printf("TrackerRequest before building URL: %+v", req)

	reqURL, err := tc.buildTrackerURL(announceURL, req)
	if err != nil {
		log.Printf("Error building tracker URL: %v", err)
		return nil, fmt.Errorf("failed to build tracker URL: %v", err)
	}

	log.Printf("Built tracker URL: %s", reqURL)

	resp, err := tc.httpClient.Get(reqURL)
	if err != nil {
		log.Printf("HTTP GET failed: %v", err)
		return nil, fmt.Errorf("tracker request failed: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Tracker response status: %s", resp.Status)

	if resp.StatusCode != http.StatusOK {
		log.Printf("Unexpected tracker status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read tracker response: %v", err)
		return nil, fmt.Errorf("failed to read tracker response: %v", err)
	}

	log.Printf("Raw response body (first 200 bytes): %s", string(body[:min(200, len(body))]))

	decoded, err := bencode.Decode(body)
	if err != nil {
		log.Printf("Bencode decode failed: %v", err)
		return nil, fmt.Errorf("failed to decode tracker response: %v", err)
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		log.Println("Decoded response is not a dictionary")
		return nil, fmt.Errorf("tracker response is not a dictionary")
	}

	log.Printf("Decoded tracker response: %+v", dict)

	respParsed, err := tc.parseTrackerResponse(dict)
	if err != nil {
		log.Printf("Failed to parse tracker response: %v", err)
		return nil, err
	}

	log.Println("Announce completed successfully")
	return respParsed, nil
}

// min helper for slicing
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
