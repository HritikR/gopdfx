package filters

import (
	"fmt"
)

// DecodeASCIIHex decodes ASCIIHexDecode data.
func DecodeASCIIHex(data []byte) ([]byte, error) {
	var decoded []byte
	var firstDigit byte
	haveFirst := false

	for i := 0; i < len(data); i++ {
		b := data[i]
		if isWhitespace(b) {
			continue
		}
		if b == '>' {
			break
		}
		if !isHexDigit(b) {
			return nil, fmt.Errorf("invalid character in ASCIIHexDecode: %c", b)
		}

		if !haveFirst {
			firstDigit = b
			haveFirst = true
		} else {
			val := hexVal(firstDigit)*16 + hexVal(b)
			decoded = append(decoded, byte(val))
			haveFirst = false
		}
	}

	// If stream ends with an odd number of hex digits, pad with a trailing 0
	if haveFirst {
		val := hexVal(firstDigit) * 16
		decoded = append(decoded, byte(val))
	}

	return decoded, nil
}

func isWhitespace(b byte) bool {
	return b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexVal(b byte) int {
	if b >= '0' && b <= '9' {
		return int(b - '0')
	}
	if b >= 'a' && b <= 'f' {
		return int(b - 'a' + 10)
	}
	if b >= 'A' && b <= 'F' {
		return int(b - 'A' + 10)
	}
	return 0
}
