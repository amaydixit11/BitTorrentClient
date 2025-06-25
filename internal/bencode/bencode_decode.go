package bencode

import (
	"errors"
	"fmt"
	"strconv"
)

type BencodeDecoder struct {
	Data []byte
	Pos  int
}

func NewDecoder(Data []byte) *BencodeDecoder {
	return &BencodeDecoder{Data: Data, Pos: 0}
}

func (d *BencodeDecoder) Decode() (interface{}, error) {

	if d.Pos >= len(d.Data) {
		return nil, errors.New("unexpected end of Data")
	}
	switch d.Data[d.Pos] {
	case 'i':
		return d.DecodeInt()
	case 'l':
		return d.DecodeList()
	case 'd':
		return d.DecodeDict()
	default:
		if d.Data[d.Pos] >= '0' && d.Data[d.Pos] <= '9' {
			return d.DecodeString()
		}
		return nil, fmt.Errorf("invalid bencode Data at position %d", d.Pos)
	}
}

// DecodeInt Decodes an integer (i<number>e)
func (d *BencodeDecoder) DecodeInt() (int64, error) {
	if d.Data[d.Pos] != 'i' {
		return 0, errors.New("expected 'i' at start of integer")
	}
	d.Pos++

	start := d.Pos
	for d.Pos < len(d.Data) && d.Data[d.Pos] != 'e' {
		d.Pos++
	}

	if d.Pos >= len(d.Data) {
		return 0, errors.New("unterminated integer")
	}

	numStr := string(d.Data[start:d.Pos])
	d.Pos++ // skip 'e'

	return strconv.ParseInt(numStr, 10, 64)
}

// DecodeString Decodes a string (<length>:<string>)
func (d *BencodeDecoder) DecodeString() (string, error) {
	start := d.Pos
	for d.Pos < len(d.Data) && d.Data[d.Pos] != ':' {
		d.Pos++
	}

	if d.Pos >= len(d.Data) {
		return "", errors.New("unterminated string length")
	}

	lengthStr := string(d.Data[start:d.Pos])
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return "", fmt.Errorf("invalid string length: %v", err)
	}

	d.Pos++ // skip ':'

	if d.Pos+length > len(d.Data) {
		return "", errors.New("string length exceeds Data")
	}

	result := string(d.Data[d.Pos : d.Pos+length])
	d.Pos += length

	return result, nil
}

// DecodeList Decodes a list (l<elements>e)
func (d *BencodeDecoder) DecodeList() ([]interface{}, error) {
	if d.Data[d.Pos] != 'l' {
		return nil, errors.New("expected 'l' at start of list")
	}
	d.Pos++

	var result []interface{}

	for d.Pos < len(d.Data) && d.Data[d.Pos] != 'e' {
		item, err := d.Decode()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	if d.Pos >= len(d.Data) {
		return nil, errors.New("unterminated list")
	}

	d.Pos++ // skip 'e'
	return result, nil
}

// DecodeDict Decodes a dictionary (d<key-value pairs>e)
func (d *BencodeDecoder) DecodeDict() (map[string]interface{}, error) {
	if d.Data[d.Pos] != 'd' {
		return nil, errors.New("expected 'd' at start of dictionary")
	}
	d.Pos++

	result := make(map[string]interface{})

	for d.Pos < len(d.Data) && d.Data[d.Pos] != 'e' {
		// Decode key (must be a string)
		key, err := d.DecodeString()
		if err != nil {
			return nil, fmt.Errorf("error decoding dictionary key: %v", err)
		}

		// Decode value
		value, err := d.Decode()
		if err != nil {
			return nil, fmt.Errorf("error decoding dictionary value: %v", err)
		}

		result[key] = value
	}

	if d.Pos >= len(d.Data) {
		return nil, errors.New("unterminated dictionary")
	}

	d.Pos++ // skip 'e'
	return result, nil
}
func Decode(Data []byte) (interface{}, error) {
	Decoder := NewDecoder(Data)
	return Decoder.Decode()
}
