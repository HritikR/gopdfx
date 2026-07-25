package filters

import (
	"fmt"
)

// DecodeASCII85 decodes ASCII85Decode data.
func DecodeASCII85(data []byte) ([]byte, error) {
	var decoded []byte
	var chunk [5]byte
	chunkIdx := 0

	for i := 0; i < len(data); i++ {
		b := data[i]
		if isWhitespace(b) {
			continue
		}
		if b == '~' {
			// Check for end marker "~>"
			if i+1 < len(data) && data[i+1] == '>' {
				break
			}
		}
		if b == 'z' {
			if chunkIdx != 0 {
				return nil, fmt.Errorf("z character inside half-completed block in ASCII85Decode")
			}
			decoded = append(decoded, 0, 0, 0, 0)
			continue
		}

		if b < '!' || b > 'u' {
			return nil, fmt.Errorf("invalid character in ASCII85Decode: %c", b)
		}

		chunk[chunkIdx] = b
		chunkIdx++

		if chunkIdx == 5 {
			var val uint64
			var pow85 uint64 = 85 * 85 * 85 * 85
			for j := 0; j < 5; j++ {
				val += uint64(chunk[j]-'!') * pow85
				if j < 4 {
					pow85 /= 85
				}
			}
			decoded = append(decoded, byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
			chunkIdx = 0
		}
	}

	if chunkIdx > 0 {
		if chunkIdx == 1 {
			return nil, fmt.Errorf("truncated ASCII85Decode block: 1 character remaining")
		}
		// Pad remaining characters with 'u' (ASCII 117)
		paddedCount := chunkIdx
		for j := chunkIdx; j < 5; j++ {
			chunk[j] = 'u'
		}

		var val uint64
		var pow85 uint64 = 85 * 85 * 85 * 85
		for j := 0; j < 5; j++ {
			val += uint64(chunk[j]-'!') * pow85
			if j < 4 {
				pow85 /= 85
			}
		}

		// Output bytes depending on initial count
		if paddedCount >= 2 {
			decoded = append(decoded, byte(val>>24))
		}
		if paddedCount >= 3 {
			decoded = append(decoded, byte(val>>16))
		}
		if paddedCount >= 4 {
			decoded = append(decoded, byte(val>>8))
		}
	}

	return decoded, nil
}
