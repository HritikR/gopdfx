package filters

import (
	"fmt"
)

// DecodeRunLength decodes RunLengthDecode data.
func DecodeRunLength(data []byte) ([]byte, error) {
	var decoded []byte
	i := 0
	for i < len(data) {
		n := data[i]
		i++
		if n == 128 {
			break
		}
		if n <= 127 {
			count := int(n) + 1
			if i+count > len(data) {
				return nil, fmt.Errorf("truncated run-length stream: literal count %d exceeds remaining bytes", count)
			}
			decoded = append(decoded, data[i:i+count]...)
			i += count
		} else {
			count := 257 - int(n)
			if i >= len(data) {
				return nil, fmt.Errorf("truncated run-length stream: missing repeated byte")
			}
			b := data[i]
			i++
			for j := 0; j < count; j++ {
				decoded = append(decoded, b)
			}
		}
	}
	return decoded, nil
}
