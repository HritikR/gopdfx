package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// DecryptAES decrypts data using AES-128 CBC with standard PKCS#7 unpadding.
// The first 16 bytes of data are treated as the Initialization Vector (IV).
func DecryptAES(key []byte, data []byte) ([]byte, error) {
	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("data is too short to contain AES IV: got %d bytes", len(data))
	}

	iv := data[:aes.BlockSize]
	ciphertext := data[aes.BlockSize:]

	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("ciphertext is empty")
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext size (%d) is not a multiple of AES block size (%d)", len(ciphertext), aes.BlockSize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	decrypted := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, ciphertext)

	unpadded, err := pkcs7Unpad(decrypted)
	if err != nil {
		return nil, err
	}

	return unpadded, nil
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("unpad error: data is empty")
	}

	padding := int(data[length-1])
	if padding < 1 || padding > aes.BlockSize {
		return nil, fmt.Errorf("unpad error: invalid padding byte value %d", padding)
	}

	for i := length - padding; i < length; i++ {
		if int(data[i]) != padding {
			return nil, fmt.Errorf("unpad error: padding bytes mismatch")
		}
	}

	return data[:length-padding], nil
}
