package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// IVSize is the size of the initialization vector for AES-GCM
	IVSize = 12
	// GCMTagSize is the size of the GCM authentication tag
	GCMTagSize = 16
)

// ValidateKeyLength validates that the key is a valid AES key size
func ValidateKeyLength(key []byte) error {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return errors.New("invalid key length: must be 16, 24, or 32 bytes")
	}
	return nil
}

// GenerateKey generates a cryptographically secure random key of the specified size
func GenerateKey(keySize int) ([]byte, error) {
	if keySize != 16 && keySize != 24 && keySize != 32 {
		return nil, errors.New("invalid key size: must be 16, 24, or 32 bytes")
	}
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// DeriveHMACKey derives an HMAC key from the base encryption key using HKDF
func DeriveHMACKey(baseKey []byte, fileID string) ([]byte, error) {
	if err := ValidateKeyLength(baseKey); err != nil {
		return nil, err
	}

	info := []byte("paste:hmac-token")
	reader := hkdf.New(sha256.New, baseKey, []byte(fileID), info)

	derived := make([]byte, len(baseKey))
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}
	return derived, nil
}

// GenerateHMACToken generates an HMAC token for file authentication
func GenerateHMACToken(fileID string, key []byte) (string, error) {
	if err := ValidateKeyLength(key); err != nil {
		return "", err
	}

	hmacKey, err := DeriveHMACKey(key, fileID)
	if err != nil {
		return "", err
	}

	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(fileID))
	signature := h.Sum(nil)

	// Truncate signature to match key size in bytes
	tokenLength := len(key)
	if tokenLength > len(signature) {
		tokenLength = len(signature)
	}
	tokenBytes := signature[:tokenLength]
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	return token, nil
}

// ParseBase64Key parses a base64-encoded key, handling missing padding
func ParseBase64Key(keyBase64 string) ([]byte, error) {
	// Add padding if needed
	if len(keyBase64)%4 != 0 {
		keyBase64 += string(make([]byte, 4-len(keyBase64)%4))
		for i := len(keyBase64) - (4 - len(keyBase64)%4); i < len(keyBase64); i++ {
			keyBase64 = keyBase64[:i] + "=" + keyBase64[i:]
		}
	}

	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, err
	}

	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}

	return key, nil
}

// StreamCipher represents a streaming encryption/decryption cipher
type StreamCipher struct {
	aead      cipher.AEAD
	iv        []byte
	chunkNum  int
	maxChunks int // Maximum number of chunks to prevent nonce reuse
}

// NewStreamCipher creates a new streaming cipher for encryption
func NewStreamCipher(key []byte) (*StreamCipher, error) {
	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	// Calculate maximum safe chunks for this IV (2^32 - 1 due to 32-bit counter)
	// But limit to reasonable value to prevent accidental nonce reuse
	maxChunks := 1 << 20 // 1 million chunks should be more than enough

	return &StreamCipher{
		aead:      aead,
		iv:        iv,
		chunkNum:  0,
		maxChunks: maxChunks,
	}, nil
}

// NewStreamDecryptor creates a new streaming cipher for decryption
func NewStreamDecryptor(key []byte, iv []byte) (*StreamCipher, error) {
	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}

	if len(iv) != IVSize {
		return nil, errors.New("invalid IV size")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	maxChunks := 1 << 20

	return &StreamCipher{
		aead:      aead,
		iv:        iv,
		chunkNum:  0,
		maxChunks: maxChunks,
	}, nil
}

// IV returns the initialization vector
func (sc *StreamCipher) IV() []byte {
	return sc.iv
}

// EncryptChunk encrypts a single chunk of data
func (sc *StreamCipher) EncryptChunk(plaintext []byte) ([]byte, error) {
	if sc.chunkNum >= sc.maxChunks {
		return nil, errors.New("maximum number of chunks reached, nonce reuse would occur")
	}

	nonce := make([]byte, IVSize)
	copy(nonce, sc.iv)
	binary.LittleEndian.PutUint32(nonce[8:], uint32(sc.chunkNum))

	ciphertext := sc.aead.Seal(nil, nonce, plaintext, nil)
	sc.chunkNum++

	return ciphertext, nil
}

// DecryptChunk decrypts a single chunk of data
func (sc *StreamCipher) DecryptChunk(ciphertext []byte) ([]byte, error) {
	if sc.chunkNum >= sc.maxChunks {
		return nil, errors.New("maximum number of chunks reached")
	}

	nonce := make([]byte, IVSize)
	copy(nonce, sc.iv)
	binary.LittleEndian.PutUint32(nonce[8:], uint32(sc.chunkNum))

	plaintext, err := sc.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	sc.chunkNum++

	return plaintext, nil
}

// Clear securely clears the cipher's sensitive data
func (sc *StreamCipher) Clear() {
	// Clear IV
	for i := range sc.iv {
		sc.iv[i] = 0
	}
	sc.chunkNum = 0
}

// EncryptMetadata encrypts metadata using AES-GCM
func EncryptMetadata(key []byte, metadata []byte) ([]byte, error) {
	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	encrypted := aead.Seal(nil, iv, metadata, nil)

	// Format: [IV(12)][Length(4)][EncryptedData]
	header := make([]byte, 16)
	copy(header[:12], iv)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(encrypted)))

	return append(header, encrypted...), nil
}

// DecryptMetadata decrypts metadata from the header format
func DecryptMetadata(key []byte, data []byte) ([]byte, error) {
	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}

	if len(data) < 16 {
		return nil, errors.New("invalid metadata format: too short")
	}

	iv := data[:12]
	metadataLen := binary.LittleEndian.Uint32(data[12:16])
	if len(data) < 16+int(metadataLen) {
		return nil, errors.New("incomplete metadata")
	}

	encryptedMetadata := data[16 : 16+metadataLen]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	decrypted, err := aead.Open(nil, iv, encryptedMetadata, nil)
	if err != nil {
		return nil, err
	}

	return decrypted, nil
}
