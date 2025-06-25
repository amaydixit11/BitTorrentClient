package tracker

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func buildURL(req TrackerRequest) (string, error) {
	u, err := url.Parse(req.AnnounceURL)
	if err != nil {
		return "", fmt.Errorf("invalid announce URL: %w", err)
	}

	q := url.Values{}
	q.Set("info_hash", PercentEncode(req.InfoHash[:]))
	q.Set("peer_id", PercentEncode(req.PeerID[:]))
	q.Set("port", strconv.Itoa(int(req.Port)))
	q.Set("uploaded", strconv.FormatInt(req.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	q.Set("left", strconv.FormatInt(req.Left, 10))
	q.Set("compact", "1")

	if req.Event != EventNone {
		q.Set("event", string(req.Event))
	}
	if req.NumWant > 0 {
		q.Set("numwant", strconv.Itoa(req.NumWant))
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Fetches tracker response and returns raw body
func Announce(req TrackerRequest) ([]byte, error) {
	trackerURL, err := buildURL(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(trackerURL)
	if err != nil {
		return nil, fmt.Errorf("tracker announce failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
