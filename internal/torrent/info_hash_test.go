package torrent

import (
	"crypto/sha1"
	"testing"
)

// The info hash is the SHA-1 of the raw, re-encoded info dictionary. Every peer
// in the swarm derives the same value, so this must match byte for byte.
func TestGenerateInfoHashMatchesSHA1(t *testing.T) {
	tor := &Torrent{}
	raw := []byte("d6:lengthi1024e4:name8:test.txt12:piece lengthi512ee")

	got := tor.GenerateInfoHash(raw)
	want := sha1.Sum(raw)

	if got != InfoHash(want) {
		t.Errorf("GenerateInfoHash = %x, want %x", got, want)
	}
}

func TestGenerateInfoHashIsDeterministic(t *testing.T) {
	tor := &Torrent{}
	raw := []byte("d4:name4:fileee")

	first := tor.GenerateInfoHash(raw)
	for i := 0; i < 10; i++ {
		if tor.GenerateInfoHash(raw) != first {
			t.Fatal("GenerateInfoHash is not deterministic")
		}
	}
}

func TestGenerateInfoHashDiffersOnDifferentInput(t *testing.T) {
	tor := &Torrent{}
	a := tor.GenerateInfoHash([]byte("d4:name1:aee"))
	b := tor.GenerateInfoHash([]byte("d4:name1:bee"))
	if a == b {
		t.Error("different info dicts produced the same info hash")
	}
}

func TestInfoHashStringIsHex(t *testing.T) {
	var ih InfoHash
	for i := range ih {
		ih[i] = byte(i)
	}
	got := ih.String()
	want := "000102030405060708090a0b0c0d0e0f10111213"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if len(got) != 40 {
		t.Errorf("hex info hash is %d chars, want 40", len(got))
	}
}

func TestValidateRejectsIncompleteTorrent(t *testing.T) {
	if err := (&Torrent{}).Validate(); err == nil {
		t.Error("empty torrent validated, want error")
	}
	if err := (&Torrent{Announce: "http://t.example/ann"}).Validate(); err == nil {
		t.Error("torrent with no info dict validated, want error")
	}
}
