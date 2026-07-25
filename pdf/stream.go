package pdf

import (
	"fmt"
	"github.com/HritikR/gopdfx/filters"
)

// Decode applies the stream's filters in reverse order and returns the decoded data.
func (s *Stream) Decode() ([]byte, error) {
	var filterNames []string
	var decodeParms []Dictionary

	// 1. Extract Filter name(s)
	if filterObj, ok := s.Dict[Name("Filter")]; ok {
		if name, ok := AsName(filterObj); ok {
			filterNames = append(filterNames, name)
		} else if arr, ok := AsArray(filterObj); ok {
			for _, item := range arr {
				if name, ok := AsName(item); ok {
					filterNames = append(filterNames, name)
				}
			}
		}
	}

	// 2. Extract DecodeParms dictionary/dictionaries
	if parmsObj, ok := s.Dict[Name("DecodeParms")]; ok {
		if dict, ok := AsDict(parmsObj); ok {
			decodeParms = append(decodeParms, dict)
		} else if arr, ok := AsArray(parmsObj); ok {
			for _, item := range arr {
				if dict, ok := AsDict(item); ok {
					decodeParms = append(decodeParms, dict)
				} else {
					decodeParms = append(decodeParms, nil)
				}
			}
		}
	}

	currentData := s.Data

	if len(filterNames) == 0 {
		return currentData, nil
	}

	// Apply decoding filters in reverse order
	for i := len(filterNames) - 1; i >= 0; i-- {
		filterName := filterNames[i]

		var parms Dictionary
		if i < len(decodeParms) {
			parms = decodeParms[i]
		}

		var err error
		switch filterName {
		case "FlateDecode", "Fl":
			predictor := 1
			columns := 1
			colors := 1
			bitsPerComponent := 8
			if parms != nil {
				predictor = getIntParam(parms, "Predictor", 1)
				columns = getIntParam(parms, "Columns", 1)
				colors = getIntParam(parms, "Colors", 1)
				bitsPerComponent = getIntParam(parms, "BitsPerComponent", 8)
			}
			currentData, err = filters.DecodeFlate(currentData, predictor, columns, colors, bitsPerComponent)

		case "ASCIIHexDecode", "AHx":
			currentData, err = filters.DecodeASCIIHex(currentData)

		case "ASCII85Decode", "A85":
			currentData, err = filters.DecodeASCII85(currentData)

		case "RunLengthDecode", "RL":
			currentData, err = filters.DecodeRunLength(currentData)

		case "LZWDecode", "LZW":
			return nil, fmt.Errorf("%w: LZWDecode is not supported", ErrUnsupportedFilter)

		default:
			return nil, fmt.Errorf("%w: filter '%s' is not supported", ErrUnsupportedFilter, filterName)
		}

		if err != nil {
			return nil, fmt.Errorf("filter '%s' decode failed: %v", filterName, err)
		}
	}

	return currentData, nil
}

func getIntParam(dict Dictionary, key string, defaultValue int) int {
	if valObj, ok := dict[Name(key)]; ok {
		if valInt, ok := AsInt(valObj); ok {
			return int(valInt)
		}
	}
	return defaultValue
}
