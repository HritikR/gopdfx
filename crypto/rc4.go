package crypto

import (
	"crypto/rc4"
)

// DecryptRC4 decrypts data using the RC4 stream cipher with the given key.
func DecryptRC4(key []byte, data []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(data))
	cipher.XORKeyStream(decrypted, data)
	return decrypted, nil
}
