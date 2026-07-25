package filters

import (
	"fmt"
)

// DecodePredictor applies TIFF or PNG predictor reconstruction.
func DecodePredictor(predictor int, data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	if predictor <= 1 {
		return data, nil
	}

	if predictor == 2 {
		return decodeTiffPredictor(data, columns, colors, bitsPerComponent)
	}

	if predictor >= 10 && predictor <= 15 {
		return decodePngPredictor(data, columns, colors, bitsPerComponent)
	}

	return nil, fmt.Errorf("unsupported predictor: %d", predictor)
}

func decodeTiffPredictor(data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	if bitsPerComponent != 8 {
		return nil, fmt.Errorf("unsupported TIFF predictor bits per component: %d (only 8-bit supported)", bitsPerComponent)
	}
	bpp := colors
	if bpp <= 0 {
		bpp = 1
	}
	rowBytes := columns * bpp

	if rowBytes <= 0 {
		return data, nil
	}

	decoded := make([]byte, len(data))
	copy(decoded, data)

	for i := 0; i < len(decoded); i += rowBytes {
		end := i + rowBytes
		if end > len(decoded) {
			end = len(decoded)
		}
		for j := i + bpp; j < end; j++ {
			decoded[j] = decoded[j] + decoded[j-bpp]
		}
	}
	return decoded, nil
}

func decodePngPredictor(data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	if bitsPerComponent != 8 {
		return nil, fmt.Errorf("unsupported PNG predictor bits per component: %d (only 8-bit supported)", bitsPerComponent)
	}

	bpp := colors
	if bpp <= 0 {
		bpp = 1
	}
	rowBytes := columns * bpp
	if rowBytes <= 0 {
		return data, nil
	}

	// Each row in the encoded stream contains a 1-byte filter prefix tag
	encodedRowLen := rowBytes + 1
	numRows := len(data) / encodedRowLen
	if len(data)%encodedRowLen != 0 {
		if len(data)%encodedRowLen > 1 {
			numRows++
		}
	}

	decoded := make([]byte, numRows*rowBytes)

	for r := 0; r < numRows; r++ {
		startEnc := r * encodedRowLen
		if startEnc >= len(data) {
			break
		}

		filterType := data[startEnc]

		encEnd := startEnc + encodedRowLen
		if encEnd > len(data) {
			encEnd = len(data)
		}

		rowEncData := data[startEnc+1 : encEnd]
		rowDecStart := r * rowBytes

		for c := 0; c < len(rowEncData); c++ {
			var left, up, upLeft byte

			if c >= bpp {
				left = decoded[rowDecStart+c-bpp]
			}
			if r > 0 {
				up = decoded[(r-1)*rowBytes+c]
			}
			if r > 0 && c >= bpp {
				upLeft = decoded[(r-1)*rowBytes+c-bpp]
			}

			encByte := rowEncData[c]
			var decByte byte

			switch filterType {
			case 0: // None
				decByte = encByte
			case 1: // Sub
				decByte = encByte + left
			case 2: // Up
				decByte = encByte + up
			case 3: // Average
				decByte = encByte + byte((int(left)+int(up))/2)
			case 4: // Paeth
				decByte = encByte + paethPredictor(left, up, upLeft)
			default:
				return nil, fmt.Errorf("unknown PNG filter type: %d", filterType)
			}

			if rowDecStart+c < len(decoded) {
				decoded[rowDecStart+c] = decByte
			}
		}
	}

	return decoded, nil
}

func paethPredictor(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := abs(p - int(a))
	pb := abs(p - int(b))
	pc := abs(p - int(c))
	if pa <= pb && pa <= pc {
		return a
	} else if pb <= pc {
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
