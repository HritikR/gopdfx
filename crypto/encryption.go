package crypto

import (
	"fmt"
)

type EncryptContext struct {
	V               int
	R               int
	O               []byte
	U               []byte
	P               int64
	Length          int // Key length in bits
	EncryptMetadata bool
	FileID          []byte
	MasterKey       []byte
	unlocked        bool
	active          bool
}

// NewEncryptContext constructs a new security handler context.
func NewEncryptContext(v, r int, o, u []byte, p int64, length int, encryptMetadata bool, fileID []byte) *EncryptContext {
	if length == 0 {
		length = 40
	}
	return &EncryptContext{
		V:               v,
		R:               r,
		O:               o,
		U:               u,
		P:               p,
		Length:          length,
		EncryptMetadata: encryptMetadata,
		FileID:          fileID,
		active:          true,
	}
}

func (c *EncryptContext) IsActive() bool {
	return c != nil && c.active
}

func (c *EncryptContext) IsUnlocked() bool {
	return c != nil && c.unlocked
}

func (c *EncryptContext) SetUnlocked(masterKey []byte) {
	c.MasterKey = masterKey
	c.unlocked = true
}

func (c *EncryptContext) DecryptBytes(data []byte, objNum, genNum int64) ([]byte, error) {
	if !c.unlocked {
		return nil, fmt.Errorf("crypt context is locked; call Unlock first")
	}
	if len(data) == 0 {
		return data, nil
	}

	key := c.DeriveObjectKey(objNum, genNum)

	if c.V == 4 {
		return DecryptAES(key, data)
	}
	return DecryptRC4(key, data)
}

func (c *EncryptContext) DeriveObjectKey(objNum, genNum int64) []byte {
	return DeriveObjectKey(c.MasterKey, c.Length/8, objNum, genNum, c.V)
}
