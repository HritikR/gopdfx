package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"testing"
)

func TestCrypto_Padding(t *testing.T) {
	padded := padPassword("secret")
	if len(padded) != 32 {
		t.Fatalf("Expected padded password length 32, got %d", len(padded))
	}
	if !bytes.HasPrefix(padded, []byte("secret")) {
		t.Errorf("Padded password does not start with original password")
	}

	paddedLong := padPassword("this-is-a-very-long-password-more-than-32-chars")
	if len(paddedLong) != 32 {
		t.Fatalf("Expected truncated padded password length 32, got %d", len(paddedLong))
	}
	if string(paddedLong) != "this-is-a-very-long-password-mor" {
		t.Errorf("Expected truncated password, got %q", string(paddedLong))
	}
}

func TestCrypto_VerifyUserPassword_Revision2(t *testing.T) {
	// Let's generate O and U for Revision 2 with user password "secret"
	p := int64(-4) // permissions
	lengthBits := 40
	r := 2
	fileID := []byte("fileID1234567890")
	encryptMetadata := true

	// 1. Generate O using owner password "ownerpass" and user password "secret"
	paddedOwner := padPassword("ownerpass")
	h := md5.New()
	h.Write(paddedOwner)
	hash := h.Sum(nil)
	ownerKey := hash[:lengthBits/8]
	o, err := DecryptRC4(ownerKey, padPassword("secret"))
	if err != nil {
		t.Fatalf("DecryptRC4 failed for O: %v", err)
	}

	// 2. Derive userKey using User password "secret" and the generated O
	userKey := DeriveUserKey("secret", o, nil, p, lengthBits, r, fileID, encryptMetadata)

	// 3. Generate U using userKey
	u, err := DecryptRC4(userKey, passwordPadding)
	if err != nil {
		t.Fatalf("DecryptRC4 failed: %v", err)
	}

	// 1. Verify User Password
	derivedKey, ok := VerifyUserPassword("secret", o, u, p, lengthBits, r, fileID, encryptMetadata)
	if !ok {
		t.Fatalf("User password verification failed")
	}
	if !bytes.Equal(derivedKey, userKey) {
		t.Errorf("Derived User key mismatch: expected %+v, got %+v", userKey, derivedKey)
	}

	// 2. Verify Wrong User Password
	_, ok = VerifyUserPassword("wrongpass", o, u, p, lengthBits, r, fileID, encryptMetadata)
	if ok {
		t.Errorf("User password verification succeeded for incorrect password")
	}

	// 3. Verify Owner Password
	derivedKey, ok = VerifyOwnerPassword("ownerpass", o, u, p, lengthBits, r, fileID, encryptMetadata)
	if !ok {
		t.Fatalf("Owner password verification failed")
	}
	if !bytes.Equal(derivedKey, userKey) {
		t.Errorf("Derived User key via owner verification mismatch")
	}
}

func TestCrypto_ObjectKeyDerivation(t *testing.T) {
	masterKey := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	keyLen := 5 // 40-bit
	objNum := int64(12)
	genNum := int64(0)

	// RC4 Object Key
	rc4Key := DeriveObjectKey(masterKey, keyLen, objNum, genNum, 1)
	if len(rc4Key) != 10 {
		t.Errorf("Expected RC4 derived key length 10 (keyLen + 5), got %d", len(rc4Key))
	}

	// AES Object Key
	aesKey := DeriveObjectKey(masterKey, keyLen, objNum, genNum, 4)
	if len(aesKey) != 10 {
		t.Errorf("Expected AES derived key length 10, got %d", len(aesKey))
	}
}

func TestCrypto_AES(t *testing.T) {
	key := []byte("aeskey1234567890") // 16 bytes
	iv := []byte("iv12345678901234")  // 16 bytes
	plaintext := []byte("Hello Encryption PKCS7")

	// PKCS#7 pad plaintext
	paddingLen := 16 - (len(plaintext) % 16)
	paddedPlaintext := make([]byte, len(plaintext)+paddingLen)
	copy(paddedPlaintext, plaintext)
	for i := len(plaintext); i < len(paddedPlaintext); i++ {
		paddedPlaintext[i] = byte(paddingLen)
	}

	// Encrypt using standard library AES-CBC to verify our DecryptAES works
	ciphertext, err := encryptAESCBC(key, iv, paddedPlaintext)
	if err != nil {
		t.Fatalf("AES encryption failed: %v", err)
	}

	// Concat IV + Ciphertext
	encryptedPayload := append(iv, ciphertext...)

	// Decrypt
	decrypted, err := DecryptAES(key, encryptedPayload)
	if err != nil {
		t.Fatalf("DecryptAES failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Expected decrypted plaintext %q, got %q", plaintext, decrypted)
	}
}

func encryptAESCBC(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, len(data))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, data)
	return ciphertext, nil
}
