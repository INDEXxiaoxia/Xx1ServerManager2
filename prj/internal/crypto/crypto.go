package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// CryptoManager 加密管理器，负责主密码派生与服务器密码加解密
type CryptoManager struct {
	masterKey []byte
}

const (
	pbkdf2Iterations = 100000
	keyLen           = 32 // AES-256
	ivLen            = 16
)

// New 创建加密管理器
func New() *CryptoManager {
	return &CryptoManager{}
}

// GenerateSalt 生成 32 字节随机盐值，返回 base64
func (c *CryptoManager) GenerateSalt() (string, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

// HashPassword 计算主密码哈希（SHA-256(密码+盐值) hex）
func (c *CryptoManager) HashPassword(password, salt string) string {
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		saltBytes = []byte(salt)
	}
	hash := sha256.Sum256(append([]byte(password), saltBytes...))
	return hex.EncodeToString(hash[:])
}

// VerifyMasterPassword 验证主密码
func (c *CryptoManager) VerifyMasterPassword(password, storedHash, salt string) bool {
	return c.HashPassword(password, salt) == storedHash
}

// SetMasterPassword 设置主密码并派生密钥
func (c *CryptoManager) SetMasterPassword(password, salt string) error {
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		return err
	}
	c.masterKey = pbkdf2.Key([]byte(password), saltBytes, pbkdf2Iterations, keyLen, sha256.New)
	return nil
}

// HasMasterKey 是否已设置主密钥
func (c *CryptoManager) HasMasterKey() bool {
	return c.masterKey != nil
}

// EncryptPassword 加密密码，返回 base64(IV+密文)，PKCS7 填充
func (c *CryptoManager) EncryptPassword(plain string) (string, error) {
	if c.masterKey == nil {
		return "", errors.New("主密钥未设置")
	}
	block, err := aes.NewCipher(c.masterKey)
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	// PKCS7 填充
	plainBytes := []byte(plain)
	padLen := aes.BlockSize - len(plainBytes)%aes.BlockSize
	padded := append(plainBytes, bytesOf(padLen, padLen)...)

	cbc := cipher.NewCBCEncrypter(block, iv)
	out := make([]byte, len(padded))
	cbc.CryptBlocks(out, padded)

	combined := append(iv, out...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptPassword 解密密码
func (c *CryptoManager) DecryptPassword(enc string) (string, error) {
	if c.masterKey == nil {
		return "", errors.New("主密钥未设置")
	}
	combined, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	if len(combined) < ivLen+aes.BlockSize {
		return "", errors.New("密文无效")
	}
	iv := combined[:ivLen]
	ct := combined[ivLen:]

	block, err := aes.NewCipher(c.masterKey)
	if err != nil {
		return "", err
	}
	cbc := cipher.NewCBCDecrypter(block, iv)
	pt := make([]byte, len(ct))
	cbc.CryptBlocks(pt, ct)
	if len(pt) == 0 {
		return "", errors.New("解密失败")
	}
	padLen := int(pt[len(pt)-1])
	if padLen > aes.BlockSize || padLen > len(pt) {
		return "", errors.New("填充无效")
	}
	return string(pt[:len(pt)-padLen]), nil
}

func bytesOf(v, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(v)
	}
	return b
}