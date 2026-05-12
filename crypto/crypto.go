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
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

// v2 format constants. Bumping these strings is a wire-format break.
const (
	chunkAAD    = "paste-v2-chunk"
	metadataAAD = "paste-v2-metadata"

	argon2Salt   = "paste-v2-argon2id"
	argon2Time   = 3
	argon2Memory = 64 * 1024 // 64 MiB
	argon2Par    = 4
	argon2Out    = 32

	hkdfFileIDInfo = "paste-v2-file-id"
	hkdfKeyInfo    = "paste-v2-encryption-key"
	hkdfHMACInfo   = "paste:hmac-token"

	streamFinalBit    uint32 = 0x80000000
	streamCounterMask uint32 = 0x7FFFFFFF

	IVSize     = 12
	GCMTagSize = 16
)

// ValidateKeyLength validates that the key is a valid AES key size.
func ValidateKeyLength(key []byte) error {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return errors.New("invalid key length: must be 16, 24, or 32 bytes")
	}
	return nil
}

// GenerateKey generates a cryptographically secure random key of the specified size.
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

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// DecodeKey decodes a base64 URL-safe key. Accepts both raw (unpadded) and
// padded forms.
func DecodeKey(s string) ([]byte, error) {
	if k, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return k, nil
	}
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.URLEncoding.DecodeString(s)
}

// DeriveHMACKey derives an HMAC key from the base encryption key using HKDF.
func DeriveHMACKey(baseKey []byte, fileID string) ([]byte, error) {
	if err := ValidateKeyLength(baseKey); err != nil {
		return nil, err
	}

	reader := hkdf.New(sha256.New, baseKey, []byte(fileID), []byte(hkdfHMACInfo))
	derived := make([]byte, len(baseKey))
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}
	return derived, nil
}

// GenerateHMACToken generates an HMAC token for file authentication.
func GenerateHMACToken(fileID string, key []byte) (string, error) {
	if err := ValidateKeyLength(key); err != nil {
		return "", err
	}

	hmacKey, err := DeriveHMACKey(key, fileID)
	if err != nil {
		return "", err
	}
	defer zero(hmacKey)

	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(fileID))
	signature := h.Sum(nil)

	tokenLength := len(key)
	if tokenLength > len(signature) {
		tokenLength = len(signature)
	}
	return base64.RawURLEncoding.EncodeToString(signature[:tokenLength]), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// buildChunkNonce writes the STREAM nonce for chunkIdx into dst.
// dst must be IVSize bytes. The high bit of the 32-bit counter encodes isFinal,
// so truncation, reordering, and extension all fail GCM authentication.
func buildChunkNonce(dst []byte, iv []byte, chunkIdx uint32, isFinal bool) {
	copy(dst, iv[:8])
	counter := chunkIdx & streamCounterMask
	if isFinal {
		counter |= streamFinalBit
	}
	binary.LittleEndian.PutUint32(dst[8:], counter)
}

// StreamCipher represents a streaming encryption/decryption cipher.
type StreamCipher struct {
	aead     cipher.AEAD
	iv       []byte
	chunkNum uint32
}

// NewStreamCipher creates a new streaming cipher for encryption with a fresh IV.
func NewStreamCipher(key []byte) (*StreamCipher, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	return &StreamCipher{aead: aead, iv: iv}, nil
}

// NewStreamDecryptor creates a new streaming cipher for decryption.
func NewStreamDecryptor(key []byte, iv []byte) (*StreamCipher, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != IVSize {
		return nil, errors.New("invalid IV size")
	}
	ivCopy := make([]byte, IVSize)
	copy(ivCopy, iv)
	return &StreamCipher{aead: aead, iv: ivCopy}, nil
}

// IV returns the initialization vector.
func (sc *StreamCipher) IV() []byte {
	return sc.iv
}

// EncryptChunk encrypts a single chunk. isFinal must be true for the final
// chunk and false otherwise — this is bound into the STREAM nonce, so a
// mismatch on decryption will fail GCM auth.
func (sc *StreamCipher) EncryptChunk(plaintext []byte, isFinal bool) ([]byte, error) {
	if sc.chunkNum >= streamCounterMask {
		return nil, errors.New("chunk counter exhausted")
	}
	nonce := make([]byte, IVSize)
	buildChunkNonce(nonce, sc.iv, sc.chunkNum, isFinal)
	ciphertext := sc.aead.Seal(nil, nonce, plaintext, []byte(chunkAAD))
	sc.chunkNum++
	return ciphertext, nil
}

// DecryptChunk decrypts a single chunk. isFinal must match the value the
// sender passed to EncryptChunk; otherwise the GCM tag fails to verify.
func (sc *StreamCipher) DecryptChunk(ciphertext []byte, isFinal bool) ([]byte, error) {
	if sc.chunkNum >= streamCounterMask {
		return nil, errors.New("chunk counter exhausted")
	}
	nonce := make([]byte, IVSize)
	buildChunkNonce(nonce, sc.iv, sc.chunkNum, isFinal)
	plaintext, err := sc.aead.Open(nil, nonce, ciphertext, []byte(chunkAAD))
	if err != nil {
		return nil, err
	}
	sc.chunkNum++
	return plaintext, nil
}

// Clear securely clears the cipher's sensitive data.
func (sc *StreamCipher) Clear() {
	zero(sc.iv)
	sc.chunkNum = 0
}

// EncryptMetadata encrypts metadata using AES-GCM with the v2 metadata AAD.
// Wire format: [IV(12)][Length(4 LE)][AES-GCM(metadata)].
func EncryptMetadata(key []byte, metadata []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	encrypted := aead.Seal(nil, iv, metadata, []byte(metadataAAD))

	header := make([]byte, 16)
	copy(header[:12], iv)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(encrypted)))
	return append(header, encrypted...), nil
}

// DecryptMetadata decrypts metadata from the v2 header format.
func DecryptMetadata(key []byte, data []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	if len(data) < 16 {
		return nil, errors.New("invalid metadata format: too short")
	}

	iv := data[:12]
	metadataLen := binary.LittleEndian.Uint32(data[12:16])
	if uint64(len(data)) < uint64(16)+uint64(metadataLen) {
		return nil, errors.New("incomplete metadata")
	}
	encryptedMetadata := data[16 : 16+metadataLen]

	return aead.Open(nil, iv, encryptedMetadata, []byte(metadataAAD))
}

// DeriveFromPassphrase derives both a file ID and encryption key from a passphrase
// using Argon2id for password stretching, then HKDF for labeled key separation.
// Returns: fileID (hex string), encryption key (bytes), error.
func DeriveFromPassphrase(passphrase string, keySize int) (string, []byte, error) {
	if keySize != 16 && keySize != 24 && keySize != 32 {
		return "", nil, errors.New("invalid key size: must be 16, 24, or 32 bytes")
	}

	stretched := argon2.IDKey([]byte(passphrase), []byte(argon2Salt),
		argon2Time, argon2Memory, argon2Par, argon2Out)
	defer zero(stretched)

	fileIDReader := hkdf.New(sha256.New, stretched, nil, []byte(hkdfFileIDInfo))
	fileIDBytes := make([]byte, 16)
	if _, err := io.ReadFull(fileIDReader, fileIDBytes); err != nil {
		return "", nil, err
	}

	keyReader := hkdf.New(sha256.New, stretched, nil, []byte(hkdfKeyInfo))
	key := make([]byte, keySize)
	if _, err := io.ReadFull(keyReader, key); err != nil {
		return "", nil, err
	}

	const digits = "0123456789abcdef"
	hexBuf := make([]byte, len(fileIDBytes)*2)
	for i, b := range fileIDBytes {
		hexBuf[i*2] = digits[b>>4]
		hexBuf[i*2+1] = digits[b&0x0f]
	}
	return string(hexBuf), key, nil
}
