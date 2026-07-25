package crypto

import (
	"bytes"
	"crypto/md5"
)

var passwordPadding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func padPassword(password string) []byte {
	pwdBytes := []byte(password)
	if len(pwdBytes) >= 32 {
		return pwdBytes[:32]
	}
	padded := make([]byte, 32)
	copy(padded, pwdBytes)
	copy(padded[len(pwdBytes):], passwordPadding)
	return padded
}

// DeriveUserKey derives the encryption key from the User password.
func DeriveUserKey(password string, o, u []byte, p int64, lengthBits int, r int, fileID []byte, encryptMetadata bool) []byte {
	padded := padPassword(password)
	return DeriveUserKeyWithPadded(padded, o, u, p, lengthBits, r, fileID, encryptMetadata)
}

// DeriveUserKeyWithPadded derives the encryption key using an already-padded 32-byte User password.
func DeriveUserKeyWithPadded(paddedUser []byte, o, u []byte, p int64, lengthBits int, r int, fileID []byte, encryptMetadata bool) []byte {
	h := md5.New()
	h.Write(paddedUser)
	h.Write(o)

	pBytes := []byte{
		byte(p),
		byte(p >> 8),
		byte(p >> 16),
		byte(p >> 24),
	}
	h.Write(pBytes)
	h.Write(fileID)

	if r >= 4 && !encryptMetadata {
		h.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	}

	hash := h.Sum(nil)
	keyLengthBytes := lengthBits / 8

	if r >= 3 {
		for i := 0; i < 50; i++ {
			h2 := md5.New()
			h2.Write(hash[:keyLengthBytes])
			hash = h2.Sum(nil)
		}
	}

	return hash[:keyLengthBytes]
}

// VerifyUserPassword attempts to authenticate the user password and returns the derived master key.
func VerifyUserPassword(password string, o, u []byte, p int64, lengthBits int, r int, fileID []byte, encryptMetadata bool) ([]byte, bool) {
	masterKey := DeriveUserKey(password, o, u, p, lengthBits, r, fileID, encryptMetadata)
	keyLen := lengthBits / 8

	if r == 2 {
		dec, err := DecryptRC4(masterKey, u)
		if err == nil && bytes.Equal(dec, passwordPadding) {
			return masterKey, true
		}
	} else if r == 3 || r == 4 {
		h := md5.New()
		h.Write(passwordPadding)
		h.Write(fileID)
		hashVal := h.Sum(nil)

		current := make([]byte, 16)
		copy(current, hashVal)

		for i := 0; i < 20; i++ {
			iterKey := make([]byte, keyLen)
			copy(iterKey, masterKey)
			if i > 0 {
				for j := 0; j < keyLen; j++ {
					iterKey[j] ^= byte(i)
				}
			}

			dec, err := DecryptRC4(iterKey, current)
			if err != nil {
				return nil, false
			}
			current = dec
		}

		if bytes.Equal(current, u[:16]) {
			return masterKey, true
		}
	}

	return nil, false
}

// VerifyOwnerPassword attempts to authenticate the owner password and returns the derived master key.
func VerifyOwnerPassword(password string, o, u []byte, p int64, lengthBits int, r int, fileID []byte, encryptMetadata bool) ([]byte, bool) {
	paddedOwner := padPassword(password)
	keyLen := lengthBits / 8

	var userPasswordPadded []byte

	if r == 2 {
		h := md5.New()
		h.Write(paddedOwner)
		hash := h.Sum(nil)
		ownerKey := hash[:keyLen]

		dec, err := DecryptRC4(ownerKey, o)
		if err != nil {
			return nil, false
		}
		userPasswordPadded = dec
	} else if r == 3 || r == 4 {
		h := md5.New()
		h.Write(paddedOwner)
		hash := h.Sum(nil)
		for i := 0; i < 50; i++ {
			h2 := md5.New()
			h2.Write(hash)
			hash = h2.Sum(nil)
		}
		ownerKey := hash[:keyLen]

		current := make([]byte, 32)
		copy(current, o)

		for i := 19; i >= 0; i-- {
			iterKey := make([]byte, keyLen)
			copy(iterKey, ownerKey)
			if i > 0 {
				for j := 0; j < keyLen; j++ {
					iterKey[j] ^= byte(i)
				}
			}

			dec, err := DecryptRC4(iterKey, current)
			if err != nil {
				return nil, false
			}
			current = dec
		}
		userPasswordPadded = current
	}

	if len(userPasswordPadded) == 32 {
		// Valid user password derived from owner password. Let's verify and get master key
		masterKey := DeriveUserKeyWithPadded(userPasswordPadded, o, u, p, lengthBits, r, fileID, encryptMetadata)

		if r == 2 {
			dec, err := DecryptRC4(masterKey, u)
			if err == nil && bytes.Equal(dec, passwordPadding) {
				return masterKey, true
			}
		} else if r == 3 || r == 4 {
			h := md5.New()
			h.Write(passwordPadding)
			h.Write(fileID)
			hashVal := h.Sum(nil)

			current := make([]byte, 16)
			copy(current, hashVal)

			for i := 0; i < 20; i++ {
				iterKey := make([]byte, keyLen)
				copy(iterKey, masterKey)
				if i > 0 {
					for j := 0; j < keyLen; j++ {
						iterKey[j] ^= byte(i)
					}
				}
				dec, err := DecryptRC4(iterKey, current)
				if err != nil {
					return nil, false
				}
				current = dec
			}

			if bytes.Equal(current, u[:16]) {
				return masterKey, true
			}
		}
	}

	return nil, false
}

// DeriveObjectKey derives the object-specific encryption key based on object and generation numbers.
func DeriveObjectKey(masterKey []byte, keyLen int, objNum, genNum int64, v int) []byte {
	h := md5.New()
	h.Write(masterKey)

	objBytes := []byte{
		byte(objNum),
		byte(objNum >> 8),
		byte(objNum >> 16),
	}
	h.Write(objBytes)

	genBytes := []byte{
		byte(genNum),
		byte(genNum >> 8),
	}
	h.Write(genBytes)

	if v == 4 {
		h.Write([]byte{0x73, 0x41, 0x6C, 0x54}) // "sAlT" salt
	}

	hash := h.Sum(nil)

	derivedKeyLen := keyLen + 5
	if derivedKeyLen > 16 {
		derivedKeyLen = 16
	}
	return hash[:derivedKeyLen]
}
