package pdf

import (
	"crypto/md5"
	"fmt"
	"github.com/HritikR/gopdfx/crypto"
	"testing"
)

var testPasswordPadding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func padTestPassword(password string) []byte {
	pwdBytes := []byte(password)
	if len(pwdBytes) >= 32 {
		return pwdBytes[:32]
	}
	padded := make([]byte, 32)
	copy(padded, pwdBytes)
	copy(padded[len(pwdBytes):], testPasswordPadding)
	return padded
}

func TestEncryption_DecryptDocument(t *testing.T) {
	p := int64(-4)
	lengthBits := 40
	r := 2
	fileID := []byte("1234567890123456") // 16 bytes

	// 1. Generate O using owner password "ownerpass" and user password "userpass"
	paddedOwner := padTestPassword("ownerpass")
	h := md5.New()
	h.Write(paddedOwner)
	hash := h.Sum(nil)
	ownerKey := hash[:lengthBits/8]
	o, _ := crypto.DecryptRC4(ownerKey, padTestPassword("userpass"))

	// 2. Derive userKey using User password "userpass" and the generated O
	userKey := crypto.DeriveUserKey("userpass", o, nil, p, lengthBits, r, fileID, true)

	// 3. Generate U using userKey
	u, _ := crypto.DecryptRC4(userKey, testPasswordPadding)

	// Encrypt the string "(SecretText)" in object 1
	objKey := crypto.DeriveObjectKey(userKey, 5, 1, 0, 1)
	encSecret, _ := crypto.DecryptRC4(objKey, []byte("SecretText"))
	hexSecret := fmt.Sprintf("%X", encSecret)

	header := "%PDF-1.4\n"
	obj1 := fmt.Sprintf("1 0 obj\n<< /Type /Catalog /Pages 2 0 R /SecString <%s> >>\nendobj\n", hexSecret)
	obj2 := "2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n"

	hexO := fmt.Sprintf("%X", o)
	hexU := fmt.Sprintf("%X", u)
	hexID := fmt.Sprintf("%X", fileID)

	obj3 := fmt.Sprintf("3 0 obj\n<< /Filter /Standard /V 1 /R 2 /Length 40 /P %d /O <%s> /U <%s> >>\nendobj\n", p, hexO, hexU)

	offset1 := len(header)
	offset2 := offset1 + len(obj1)
	offset3 := offset2 + len(obj2)
	offsetXref := offset3 + len(obj3)

	xrefTable := fmt.Sprintf("xref\n0 4\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n", offset1, offset2, offset3)
	trailer := fmt.Sprintf("trailer\n<< /Size 4 /Root 1 0 R /Encrypt 3 0 R /ID [ <%s> <%s> ] >>\nstartxref\n%d\n%%EOF", hexID, hexID, offsetXref)

	pdfBytes := []byte(header + obj1 + obj2 + obj3 + xrefTable + trailer)

	// Test Open and Decryption
	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}

	if doc.Encryption == nil {
		t.Fatal("Expected Encryption context to be set")
	}
	if doc.Encryption.IsUnlocked() {
		t.Fatal("Expected document to be locked initially")
	}

	// Resolve catalog before unlock (should return encrypted/raw text, not "SecretText")
	catObj, err := doc.Resolve(IndirectRef{ObjectNumber: 1, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	catDict := catObj.(Dictionary)
	secStr, _ := AsString(catDict[Name("SecString")])
	if secStr == "SecretText" {
		t.Errorf("Catalog decrypted before unlock")
	}

	// Try Unlock with wrong password
	err = doc.Unlock("wrongpass")
	if err != ErrWrongPassword {
		t.Errorf("Expected ErrWrongPassword, got %v", err)
	}

	// Unlock with user password
	err = doc.Unlock("userpass")
	if err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
	if !doc.Encryption.IsUnlocked() {
		t.Fatal("Expected unlocked to be true")
	}

	// Clear cache so that it resolves again and decrypts
	delete(doc.objectCache, 1)

	catObj, err = doc.Resolve(IndirectRef{ObjectNumber: 1, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	catDict = catObj.(Dictionary)
	secStr, _ = AsString(catDict[Name("SecString")])
	if secStr != "SecretText" {
		t.Errorf("Expected 'SecretText', got %q", secStr)
	}
}
