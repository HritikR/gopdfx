package pdf

import "errors"

var (
	ErrInvalidPDF            = errors.New("invalid PDF document structure")
	ErrUnsupportedVersion    = errors.New("unsupported PDF version")
	ErrEncryptedPDF          = errors.New("pdf is encrypted")
	ErrWrongPassword         = errors.New("incorrect password")
	ErrUnsupportedEncryption = errors.New("unsupported encryption algorithm")
	ErrCorruptObject         = errors.New("corrupt object or parsing failure")
	ErrUnsupportedFilter     = errors.New("unsupported stream filter")
	ErrFontDecodeFailure     = errors.New("failed to decode font data")
)
