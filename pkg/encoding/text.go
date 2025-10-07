package encoding

import (
	"errors"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

// TextEncoder handles encoding and decoding of SMS text in various formats
type TextEncoder struct{}

// NewTextEncoder creates a new text encoder
func NewTextEncoder() *TextEncoder {
	return &TextEncoder{}
}

// GSM 7-bit alphabet mapping
var gsm7BitAlphabet = map[rune]byte{
	'@': 0x00, '£': 0x01, '$': 0x02, '¥': 0x03, 'è': 0x04, 'é': 0x05, 'ù': 0x06, 'ì': 0x07,
	'ò': 0x08, 'Ç': 0x09, '\n': 0x0A, 'Ø': 0x0B, 'ø': 0x0C, '\r': 0x0D, 'Å': 0x0E, 'å': 0x0F,
	'Δ': 0x10, '_': 0x11, 'Φ': 0x12, 'Γ': 0x13, 'Λ': 0x14, 'Ω': 0x15, 'Π': 0x16, 'Ψ': 0x17,
	'Σ': 0x18, 'Θ': 0x19, 'Ξ': 0x1A, '\x1B': 0x1B, 'Æ': 0x1C, 'æ': 0x1D, 'ß': 0x1E, 'É': 0x1F,
	' ': 0x20, '!': 0x21, '"': 0x22, '#': 0x23, '¤': 0x24, '%': 0x25, '&': 0x26, '\'': 0x27,
	'(': 0x28, ')': 0x29, '*': 0x2A, '+': 0x2B, ',': 0x2C, '-': 0x2D, '.': 0x2E, '/': 0x2F,
	'0': 0x30, '1': 0x31, '2': 0x32, '3': 0x33, '4': 0x34, '5': 0x35, '6': 0x36, '7': 0x37,
	'8': 0x38, '9': 0x39, ':': 0x3A, ';': 0x3B, '<': 0x3C, '=': 0x3D, '>': 0x3E, '?': 0x3F,
	'¡': 0x40, 'A': 0x41, 'B': 0x42, 'C': 0x43, 'D': 0x44, 'E': 0x45, 'F': 0x46, 'G': 0x47,
	'H': 0x48, 'I': 0x49, 'J': 0x4A, 'K': 0x4B, 'L': 0x4C, 'M': 0x4D, 'N': 0x4E, 'O': 0x4F,
	'P': 0x50, 'Q': 0x51, 'R': 0x52, 'S': 0x53, 'T': 0x54, 'U': 0x55, 'V': 0x56, 'W': 0x57,
	'X': 0x58, 'Y': 0x59, 'Z': 0x5A, 'Ä': 0x5B, 'Ö': 0x5C, 'Ñ': 0x5D, 'Ü': 0x5E, '§': 0x5F,
	'¿': 0x60, 'a': 0x61, 'b': 0x62, 'c': 0x63, 'd': 0x64, 'e': 0x65, 'f': 0x66, 'g': 0x67,
	'h': 0x68, 'i': 0x69, 'j': 0x6A, 'k': 0x6B, 'l': 0x6C, 'm': 0x6D, 'n': 0x6E, 'o': 0x6F,
	'p': 0x70, 'q': 0x71, 'r': 0x72, 's': 0x73, 't': 0x74, 'u': 0x75, 'v': 0x76, 'w': 0x77,
	'x': 0x78, 'y': 0x79, 'z': 0x7A, 'ä': 0x7B, 'ö': 0x7C, 'ñ': 0x7D, 'ü': 0x7E, 'à': 0x7F,
}

// Extended GSM 7-bit characters (prefixed with ESC 0x1B)
var gsm7BitExtended = map[rune]byte{
	'\f': 0x0A, // Form Feed
	'^':  0x14, // Circumflex
	'{':  0x28, // Left Brace
	'}':  0x29, // Right Brace
	'\\': 0x2F, // Backslash
	'[':  0x3C, // Left Bracket
	'~':  0x3D, // Tilde
	']':  0x3E, // Right Bracket
	'|':  0x40, // Pipe
	'€':  0x65, // Euro
}

// Reverse mapping for decoding
var gsm7BitReverse map[byte]rune
var gsm7BitExtendedReverse map[byte]rune

func init() {
	// Initialize reverse mappings
	gsm7BitReverse = make(map[byte]rune)
	for r, b := range gsm7BitAlphabet {
		gsm7BitReverse[b] = r
	}

	gsm7BitExtendedReverse = make(map[byte]rune)
	for r, b := range gsm7BitExtended {
		gsm7BitExtendedReverse[b] = r
	}
}

// EncodeGSM7Bit encodes a string to GSM 7-bit format
func (e *TextEncoder) EncodeGSM7Bit(text string) ([]byte, error) {
	var result []byte

	for _, r := range text {
		if b, ok := gsm7BitAlphabet[r]; ok {
			result = append(result, b)
		} else if b, ok := gsm7BitExtended[r]; ok {
			result = append(result, 0x1B, b) // Add escape character
		} else {
			// Character not supported in GSM 7-bit, use '?'
			result = append(result, gsm7BitAlphabet['?'])
		}
	}

	return result, nil
}

// DecodeGSM7Bit decodes GSM 7-bit format to string
func (e *TextEncoder) DecodeGSM7Bit(data []byte) (string, error) {
	var result []rune

	for i := 0; i < len(data); i++ {
		b := data[i]

		if b == 0x1B && i+1 < len(data) {
			// Extended character
			nextByte := data[i+1]
			if r, ok := gsm7BitExtendedReverse[nextByte]; ok {
				result = append(result, r)
				i++ // Skip next byte
			} else {
				// Invalid extended character, use space
				result = append(result, ' ')
				i++
			}
		} else if r, ok := gsm7BitReverse[b]; ok {
			result = append(result, r)
		} else {
			// Invalid character, use space
			result = append(result, ' ')
		}
	}

	return string(result), nil
}

// EncodeUCS2 encodes a string to UCS2 (UTF-16 Big Endian) format
func (e *TextEncoder) EncodeUCS2(text string) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("invalid UTF-8 string")
	}

	// Convert to UTF-16
	utf16Codes := utf16.Encode([]rune(text))

	// Convert to big-endian bytes
	result := make([]byte, len(utf16Codes)*2)
	for i, code := range utf16Codes {
		result[i*2] = byte(code >> 8)     // High byte
		result[i*2+1] = byte(code & 0xFF) // Low byte
	}

	return result, nil
}

// DecodeUCS2 decodes UCS2 (UTF-16 Big Endian) format to string
func (e *TextEncoder) DecodeUCS2(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("UCS2 data must have even length")
	}

	// Convert bytes to UTF-16 codes
	utf16Codes := make([]uint16, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		utf16Codes[i/2] = uint16(data[i])<<8 | uint16(data[i+1])
	}

	// Decode UTF-16 to runes
	runes := utf16.Decode(utf16Codes)

	return string(runes), nil
}

// Encode encodes text based on the specified data coding
func (e *TextEncoder) Encode(text string, dataCoding uint8) ([]byte, error) {
	switch dataCoding {
	case 0x00: // GSM 7-bit default alphabet
		return e.EncodeGSM7Bit(text)
	case 0x08: // UCS2
		return e.EncodeUCS2(text)
	case 0x04: // Binary (8-bit)
		return []byte(text), nil
	default:
		return nil, fmt.Errorf("unsupported data coding: 0x%02X", dataCoding)
	}
}

// Decode decodes text based on the specified data coding
func (e *TextEncoder) Decode(data []byte, dataCoding uint8) (string, error) {
	switch dataCoding {
	case 0x00: // GSM 7-bit default alphabet
		return e.DecodeGSM7Bit(data)
	case 0x08: // UCS2
		return e.DecodeUCS2(data)
	case 0x04: // Binary (8-bit)
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported data coding: 0x%02X", dataCoding)
	}
}

// DetectOptimalEncoding detects the optimal encoding for a given text
func (e *TextEncoder) DetectOptimalEncoding(text string) uint8 {
	// Check if text can be encoded in GSM 7-bit
	canUseGSM7Bit := true
	for _, r := range text {
		if _, ok := gsm7BitAlphabet[r]; !ok {
			if _, ok := gsm7BitExtended[r]; !ok {
				canUseGSM7Bit = false
				break
			}
		}
	}

	if canUseGSM7Bit {
		return 0x00 // GSM 7-bit
	}

	// Check if text contains only ASCII
	isASCII := true
	for _, r := range text {
		if r > 127 {
			isASCII = false
			break
		}
	}

	if isASCII {
		return 0x04 // 8-bit binary for extended ASCII
	}

	// Use UCS2 for unicode text
	return 0x08
}

// GetMaxLength returns the maximum number of characters for a given encoding in a single SMS
func (e *TextEncoder) GetMaxLength(dataCoding uint8, hasUDH bool) int {
	baseLength := 160 // GSM 7-bit default

	switch dataCoding {
	case 0x00: // GSM 7-bit
		baseLength = 160
	case 0x04: // 8-bit binary
		baseLength = 140
	case 0x08: // UCS2
		baseLength = 70
	}

	// Reduce available space if UDH (User Data Header) is present
	if hasUDH {
		switch dataCoding {
		case 0x00: // GSM 7-bit
			baseLength = 153 // 160 - 7 (typical UDH length)
		case 0x04: // 8-bit binary
			baseLength = 134 // 140 - 6
		case 0x08: // UCS2
			baseLength = 67 // 70 - 3
		}
	}

	return baseLength
}

// SplitMessage splits a message into parts if it exceeds the maximum length
func (e *TextEncoder) SplitMessage(text string, dataCoding uint8) ([]string, error) {
	maxLength := e.GetMaxLength(dataCoding, true) // Assume UDH for multipart

	if len(text) <= maxLength {
		return []string{text}, nil
	}

	var parts []string
	runes := []rune(text)

	for len(runes) > 0 {
		var part []rune
		if len(runes) <= maxLength {
			part = runes
			runes = nil
		} else {
			part = runes[:maxLength]
			runes = runes[maxLength:]
		}
		parts = append(parts, string(part))
	}

	return parts, nil
}

// ValidateText validates if text can be encoded with the specified data coding
func (e *TextEncoder) ValidateText(text string, dataCoding uint8) error {
	switch dataCoding {
	case 0x00: // GSM 7-bit
		for _, r := range text {
			if _, ok := gsm7BitAlphabet[r]; !ok {
				if _, ok := gsm7BitExtended[r]; !ok {
					return fmt.Errorf("character '%c' (U+%04X) not supported in GSM 7-bit encoding", r, r)
				}
			}
		}
	case 0x08: // UCS2
		if !utf8.ValidString(text) {
			return errors.New("invalid UTF-8 string for UCS2 encoding")
		}
	}

	return nil
}

// CalculateEncodedLength calculates the encoded length in bytes
func (e *TextEncoder) CalculateEncodedLength(text string, dataCoding uint8) (int, error) {
	encoded, err := e.Encode(text, dataCoding)
	if err != nil {
		return 0, err
	}
	return len(encoded), nil
}

// EstimatePartCount estimates how many SMS parts will be needed
func (e *TextEncoder) EstimatePartCount(text string, dataCoding uint8) int {
	maxSingle := e.GetMaxLength(dataCoding, false)
	maxMulti := e.GetMaxLength(dataCoding, true)

	textLen := len([]rune(text))

	if textLen <= maxSingle {
		return 1
	}

	return (textLen + maxMulti - 1) / maxMulti // Ceiling division
}
