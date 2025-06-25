package bencode

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
)

// BencodeEncoder handles encoding data to bencode format
type BencodeEncoder struct{}

// NewEncoder creates a new bencode encoder
func NewEncoder() *BencodeEncoder {
	return &BencodeEncoder{}
}

// Encode encodes a value to bencode format
func (e *BencodeEncoder) Encode(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case int:
		return e.encodeInt(int64(v)), nil
	case int64:
		return e.encodeInt(v), nil
	case string:
		return e.encodeString(v), nil
	case []interface{}:
		return e.encodeList(v)
	case map[string]interface{}:
		return e.encodeDict(v)
	default:
		// Use reflection for other types
		return e.encodeReflect(reflect.ValueOf(value))
	}
}

// encodeInt encodes an integer
func (e *BencodeEncoder) encodeInt(value int64) []byte {
	return []byte(fmt.Sprintf("i%de", value))
}

// encodeString encodes a string
func (e *BencodeEncoder) encodeString(value string) []byte {
	return []byte(fmt.Sprintf("%d:%s", len(value), value))
}

// encodeList encodes a list
func (e *BencodeEncoder) encodeList(value []interface{}) ([]byte, error) {
	var result []byte
	result = append(result, 'l')

	for _, item := range value {
		encoded, err := e.Encode(item)
		if err != nil {
			return nil, err
		}
		result = append(result, encoded...)
	}

	result = append(result, 'e')
	return result, nil
}

// encodeDict encodes a dictionary
func (e *BencodeEncoder) encodeDict(value map[string]interface{}) ([]byte, error) {
	var result []byte
	result = append(result, 'd')

	// Sort keys for consistent output
	keys := make([]string, 0, len(value))
	for k := range value {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		// Encode key
		keyEncoded := e.encodeString(key)
		result = append(result, keyEncoded...)

		// Encode value
		valueEncoded, err := e.Encode(value[key])
		if err != nil {
			return nil, err
		}
		result = append(result, valueEncoded...)
	}

	result = append(result, 'e')
	return result, nil
}

// encodeReflect handles encoding using reflection
func (e *BencodeEncoder) encodeReflect(v reflect.Value) ([]byte, error) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.encodeInt(v.Int()), nil
	case reflect.String:
		return e.encodeString(v.String()), nil
	case reflect.Slice, reflect.Array:
		var items []interface{}
		for i := 0; i < v.Len(); i++ {
			items = append(items, v.Index(i).Interface())
		}
		return e.encodeList(items)
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, errors.New("map keys must be strings")
		}
		dict := make(map[string]interface{})
		for _, key := range v.MapKeys() {
			dict[key.String()] = v.MapIndex(key).Interface()
		}
		return e.encodeDict(dict)
	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Type())
	}
}

func Encode(value interface{}) ([]byte, error) {
	encoder := NewEncoder()
	return encoder.Encode(value)
}
