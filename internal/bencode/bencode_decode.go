package bencode

import (
	"errors"
	"fmt"
	"strconv"
)

type BencodeDecoder struct {
	data []byte
	pos  int
}

func NewDecoder(data []byte) *BencodeDecoder {
	return &BencodeDecoder{data: data, pos: 0}
}

func (d *BencodeDecoder) Decode() (interface{}, error) {

	if d.pos >= len(d.data) {
		return nil, errors.New("unexpected end of data")
	}
	switch d.data[d.pos] {
	case 'i':
		return d.decodeInt()
	case 'l':
		return d.decodeList()
	case 'd':
		return d.decodeDict()
	default:
		if d.data[d.pos] >= '0' && d.data[d.pos] <= '9' {
			return d.decodeString()
		}
		return nil, fmt.Errorf("invalid bencode data at position %d", d.pos)
	}
}

// decodeInt decodes an integer (i<number>e)
func (d *BencodeDecoder) decodeInt() (int64, error) {
	if d.data[d.pos] != 'i' {
		return 0, errors.New("expected 'i' at start of integer")
	}
	d.pos++

	start := d.pos
	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return 0, errors.New("unterminated integer")
	}

	numStr := string(d.data[start:d.pos])
	d.pos++ // skip 'e'

	return strconv.ParseInt(numStr, 10, 64)
}

// decodeString decodes a string (<length>:<string>)
func (d *BencodeDecoder) decodeString() (string, error) {
	start := d.pos
	for d.pos < len(d.data) && d.data[d.pos] != ':' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return "", errors.New("unterminated string length")
	}

	lengthStr := string(d.data[start:d.pos])
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return "", fmt.Errorf("invalid string length: %v", err)
	}

	d.pos++ // skip ':'

	if d.pos+length > len(d.data) {
		return "", errors.New("string length exceeds data")
	}

	result := string(d.data[d.pos : d.pos+length])
	d.pos += length

	return result, nil
}

// decodeList decodes a list (l<elements>e)
func (d *BencodeDecoder) decodeList() ([]interface{}, error) {
	if d.data[d.pos] != 'l' {
		return nil, errors.New("expected 'l' at start of list")
	}
	d.pos++

	var result []interface{}

	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		item, err := d.Decode()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	if d.pos >= len(d.data) {
		return nil, errors.New("unterminated list")
	}

	d.pos++ // skip 'e'
	return result, nil
}

// decodeDict decodes a dictionary (d<key-value pairs>e)
func (d *BencodeDecoder) decodeDict() (map[string]interface{}, error) {
	if d.data[d.pos] != 'd' {
		return nil, errors.New("expected 'd' at start of dictionary")
	}
	d.pos++

	result := make(map[string]interface{})

	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		// Decode key (must be a string)
		key, err := d.decodeString()
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

	if d.pos >= len(d.data) {
		return nil, errors.New("unterminated dictionary")
	}

	d.pos++ // skip 'e'
	return result, nil
}
func Decode(data []byte) (interface{}, error) {
	decoder := NewDecoder(data)
	return decoder.Decode()
}
