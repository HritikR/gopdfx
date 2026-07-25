package filters

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
)

// DecodeFlate decompresses FlateDecode data, falling back to raw deflate if needed,
// and applies any predictor logic defined by the arguments.
func DecodeFlate(data []byte, predictor int, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	// Try standard zlib first
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err == nil {
		defer zr.Close()
		decompressed, err := io.ReadAll(zr)
		if err == nil {
			if predictor > 1 {
				return DecodePredictor(predictor, decompressed, columns, colors, bitsPerComponent)
			}
			return decompressed, nil
		}
	}

	// Fallback to raw flate (no headers)
	fr := flate.NewReader(bytes.NewReader(data))
	defer fr.Close()
	decompressed, err := io.ReadAll(fr)
	if err != nil {
		return nil, fmt.Errorf("flate decompression failed: %v", err)
	}

	if predictor > 1 {
		return DecodePredictor(predictor, decompressed, columns, colors, bitsPerComponent)
	}
	return decompressed, nil
}
