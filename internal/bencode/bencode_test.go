package bencode

import (
	"reflect"
	"testing"
)

func TestDecodeInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"i0e", 0},
		{"i42e", 42},
		{"i-42e", -42},
		{"i9223372036854775807e", 9223372036854775807},
	}
	for _, c := range cases {
		got, err := Decode([]byte(c.in))
		if err != nil {
			t.Fatalf("Decode(%q) returned error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Decode(%q) = %v, want %d", c.in, got, c.want)
		}
	}
}

func TestDecodeString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0:", ""},
		{"4:spam", "spam"},
		{"19:BitTorrent protocol", "BitTorrent protocol"},
		{"3:i3e", "i3e"}, // digits inside a string must not be re-parsed
	}
	for _, c := range cases {
		got, err := Decode([]byte(c.in))
		if err != nil {
			t.Fatalf("Decode(%q) returned error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Decode(%q) = %v, want %q", c.in, got, c.want)
		}
	}
}

func TestDecodeList(t *testing.T) {
	got, err := Decode([]byte("l4:spami42ee"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []interface{}{"spam", int64(42)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestDecodeNestedList(t *testing.T) {
	got, err := Decode([]byte("lli1ei2eel3:abcee"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []interface{}{
		[]interface{}{int64(1), int64(2)},
		[]interface{}{"abc"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestDecodeEmptyListAndDict(t *testing.T) {
	l, err := Decode([]byte("le"))
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if v, ok := l.([]interface{}); !ok || len(v) != 0 {
		t.Errorf("empty list decoded as %#v", l)
	}

	d, err := Decode([]byte("de"))
	if err != nil {
		t.Fatalf("empty dict: %v", err)
	}
	if v, ok := d.(map[string]interface{}); !ok || len(v) != 0 {
		t.Errorf("empty dict decoded as %#v", d)
	}
}

func TestDecodeDict(t *testing.T) {
	got, err := Decode([]byte("d3:bar4:spam3:fooi42ee"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{"bar": "spam", "foo": int64(42)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// A .torrent file is a dict with an "announce" URL and a nested "info" dict.
// This is the shape the parser actually has to survive.
func TestDecodeTorrentShapedDict(t *testing.T) {
	raw := "d8:announce30:http://tracker.example.com/ann4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi512eee"
	got, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	top, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("top level is %T, want map", got)
	}
	if top["announce"] != "http://tracker.example.com/ann" {
		t.Errorf("announce = %v", top["announce"])
	}
	info, ok := top["info"].(map[string]interface{})
	if !ok {
		t.Fatalf("info is %T, want map", top["info"])
	}
	if info["name"] != "test.txt" {
		t.Errorf("info.name = %v", info["name"])
	}
	if info["length"] != int64(1024) {
		t.Errorf("info.length = %v, want 1024", info["length"])
	}
	if info["piece length"] != int64(512) {
		t.Errorf("info.piece length = %v, want 512", info["piece length"])
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty input", ""},
		{"unterminated integer", "i42"},
		{"unterminated list", "l4:spam"},
		{"unterminated dict", "d3:foo"},
		{"string longer than data", "10:short"},
		{"unterminated string length", "12"},
		{"invalid leading byte", "x"},
		{"non-numeric integer", "iabce"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Decode([]byte(c.in)); err == nil {
				t.Errorf("Decode(%q) = nil error, want error", c.in)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{int64(42), "i42e"},
		{int(-7), "i-7e"},
		{"spam", "4:spam"},
		{"", "0:"},
		{[]interface{}{"spam", int64(42)}, "l4:spami42ee"},
		{map[string]interface{}{"foo": int64(42)}, "d3:fooi42ee"},
	}
	for _, c := range cases {
		got, err := Encode(c.in)
		if err != nil {
			t.Fatalf("Encode(%#v) returned error: %v", c.in, err)
		}
		if string(got) != c.want {
			t.Errorf("Encode(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// BEP-3 requires dictionary keys to be sorted; the info-hash depends on it.
func TestEncodeDictKeysAreSorted(t *testing.T) {
	in := map[string]interface{}{
		"zebra":  int64(1),
		"apple":  int64(2),
		"middle": int64(3),
	}
	got, err := Encode(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "d5:applei2e6:middlei3e5:zebrai1ee"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	in := map[string]interface{}{"b": int64(2), "a": int64(1), "c": "x"}
	first, err := Encode(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 50; i++ {
		next, err := Encode(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(next) != string(first) {
			t.Fatalf("encoding not deterministic: %q vs %q", first, next)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	cases := []interface{}{
		int64(0),
		int64(-99),
		"hello world",
		[]interface{}{int64(1), "two", []interface{}{int64(3)}},
		map[string]interface{}{
			"announce": "http://tracker.example.com/announce",
			"info": map[string]interface{}{
				"name":         "file.bin",
				"length":       int64(65536),
				"piece length": int64(16384),
			},
		},
	}
	for _, want := range cases {
		encoded, err := Encode(want)
		if err != nil {
			t.Fatalf("Encode(%#v) returned error: %v", want, err)
		}
		got, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(%q) returned error: %v", encoded, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip changed value:\n got %#v\nwant %#v", got, want)
		}
	}
}

func TestEncodeUnsupportedType(t *testing.T) {
	if _, err := Encode(3.14); err == nil {
		t.Error("Encode(float) = nil error, want error")
	}
}

func TestDecoderPositionAdvances(t *testing.T) {
	d := NewDecoder([]byte("i1ei2e"))
	first, err := d.Decode()
	if err != nil {
		t.Fatalf("first decode: %v", err)
	}
	second, err := d.Decode()
	if err != nil {
		t.Fatalf("second decode: %v", err)
	}
	if first != int64(1) || second != int64(2) {
		t.Errorf("got %v then %v, want 1 then 2", first, second)
	}
}

func FuzzDecodeDoesNotPanic(f *testing.F) {
	seeds := []string{"i42e", "4:spam", "l4:spami42ee", "d3:fooi42ee", "de", "le", "", "i", "d"}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Malformed input must return an error, never panic.
		_, _ = Decode(data)
	})
}
