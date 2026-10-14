package client

import (
	"net/url"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// EncodePassword mirrors $.su.orgAuthPwd, including JavaScript UTF-16 indexing.
func EncodePassword(password string) string {
	key := utf16.Encode([]rune("RDpbLfCPsJZ7fiv"))
	input := utf16.Encode([]rune(password))
	const dictionary = "yLwVl0zKqws7LgKPRQ84Mdt708T1qQ3Ha7xv3H7NyU84p21BriUWBU43odz3iP4rBL3cD02KZciX" +
		"TysVXiV8ngg6vL48rPJyAUw0HurW20xqxv9aYb4M9wK1Ae0wlro510qXeU07kV57fQMc8L6aLgML" +
		"wygtc0F10a0Dg70TOoouyFhdysuRMO51yY5ZlOZZLEal1h0t9YQW0Ko7oBwmCAHoic4HYbUyVeU3" +
		"sfQ1xtXcPcf1aT303wAQhv66qzW"
	length := max(len(input), len(key))
	result := make([]byte, length)
	for i := range length {
		left, right := uint16(0xBB), uint16(0xBB)
		if i < len(input) {
			left = input[i]
		}
		if i < len(key) {
			right = key[i]
		}
		result[i] = dictionary[int(left^right)%len(dictionary)]
	}
	return string(result)
}

// encodeComponent matches encodeURIComponent instead of form/url encoding.
func encodeComponent(value string) string {
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-_.!~*'()", rune(b)) {
			out.WriteByte(b)
		} else {
			out.WriteByte('%')
			out.WriteByte(hex[b>>4])
			out.WriteByte(hex[b&15])
		}
	}
	return out.String()
}

// transformStrings only transforms values; SLP object keys are not URI encoded.
func transformStrings(value any, encode bool) any {
	switch current := value.(type) {
	case string:
		if encode {
			return encodeComponent(current)
		}
		// PathUnescape preserves literal '+' as decodeURIComponent does.
		decoded, err := url.PathUnescape(current)
		if err != nil || !utf8.ValidString(decoded) {
			return current // The web UI likewise keeps malformed percent escapes.
		}
		return decoded
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, item := range current {
			out[key] = transformStrings(item, encode)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for i, item := range current {
			out[i] = transformStrings(item, encode)
		}
		return out
	default:
		return value
	}
}
