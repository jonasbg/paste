package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall/js"

	"golang.org/x/crypto/hkdf"
)

type Metadata struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type StreamingCipher struct {
	gcm   cipher.AEAD
	iv    []byte
	chunk int
}

type CipherRegistry struct {
	mu      sync.Mutex
	ciphers map[int]*StreamingCipher
	nextID  int
}

var registry = &CipherRegistry{
	ciphers: make(map[int]*StreamingCipher),
	nextID:  1,
}

func main() {
	c := make(chan struct{}, 0)
	js.Global().Set("goEncryption", map[string]interface{}{
		"createEncryptionStream": js.FuncOf(createEncryptionStream),
		"createDecryptionStream": js.FuncOf(createDecryptionStream),
		"encryptChunk":           js.FuncOf(encryptChunk),
		"decryptChunk":           js.FuncOf(decryptChunk),
		"disposeCipher":          js.FuncOf(disposeCipher),
		"generateKey":            js.FuncOf(generateKey),
		"decryptMetadata":        js.FuncOf(decryptMetadata),
		"encrypt":                js.FuncOf(encrypt),
		"generateHmacToken":      js.FuncOf(generateHmacToken),
	})
	<-c
}

func generateHmacToken(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	fileId := args[0].String()
	keyBase64 := args[1].String()

	// Ensure proper padding before decoding the base64 key
	if len(keyBase64)%4 != 0 {
		keyBase64 += strings.Repeat("=", 4-len(keyBase64)%4)
	}

	// Decode the base64 key
	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return handleError(errors.New("invalid key length"))
	}

	hmacKey, err := deriveHMACKey(key, fileId)
	if err != nil {
		return handleError(err)
	}

	// Create HMAC with derived key
	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(fileId))
	signature := h.Sum(nil)

	// Truncate signature to match key size in bytes (16, 24, or 32)
	tokenLength := len(key)
	if tokenLength > len(signature) {
		tokenLength = len(signature)
	}
	tokenBytes := signature[:tokenLength]
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// Validate the token is filename safe
	safeChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	for _, char := range token {
		if !strings.ContainsRune(safeChars, char) {
			return handleError(errors.New("generated token contains unsafe characters"))
		}
	}

	return js.ValueOf(token)
}

func deriveHMACKey(baseKey []byte, fileID string) ([]byte, error) {
	info := []byte("paste:hmac-token")
	reader := hkdf.New(sha256.New, baseKey, []byte(fileID), info)

	derived := make([]byte, len(baseKey))
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}
	return derived, nil
}

func encrypt(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	// Decode the key using base64 URL-safe encoding
	keyBase64 := args[0].String()

	// Handle base64 padding (add '=' if it's missing)
	if len(keyBase64)%4 != 0 {
		keyBase64 += strings.Repeat("=", 4-len(keyBase64)%4)
	}

	// Decode the base64 encoded key (using URL-safe base64)
	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	// Validate key length (must be 16, 24, or 32 bytes)
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return handleError(errors.New("invalid key length"))
	}

	// Copy the data into a byte slice
	data := make([]byte, args[1].Length())
	js.CopyBytesToGo(data, args[1])

	// Generate a 12-byte IV for GCM mode
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return handleError(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return handleError(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return handleError(err)
	}

	encrypted := aead.Seal(nil, iv, data, nil)

	result := append(iv, encrypted...)

	uint8Array := js.Global().Get("Uint8Array").New(len(result))
	js.CopyBytesToJS(uint8Array, result)
	return uint8Array
}

func createEncryptionStream(_ js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		return handleError(errors.New("invalid arguments"))
	}

	keyBase64 := args[0].String()
	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return handleError(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return handleError(err)
	}

	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return handleError(err)
	}

	cipher := &StreamingCipher{
		gcm:   aead,
		iv:    iv,
		chunk: 0,
	}

	// Register cipher and get ID
	registry.mu.Lock()
	cipherID := registry.nextID
	registry.nextID++
	registry.ciphers[cipherID] = cipher
	registry.mu.Unlock()

	// Return both cipher ID and IV
	uint8Array := js.Global().Get("Uint8Array").New(len(iv))
	js.CopyBytesToJS(uint8Array, iv)

	return js.ValueOf(map[string]interface{}{
		"id": cipherID,
		"iv": uint8Array,
	})
}

func createDecryptionStream(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	keyBase64 := args[0].String()
	iv := make([]byte, args[1].Length())
	js.CopyBytesToGo(iv, args[1])

	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return handleError(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return handleError(err)
	}

	cipher := &StreamingCipher{
		gcm:   aead,
		iv:    iv,
		chunk: 0,
	}

	// Register cipher and get ID
	registry.mu.Lock()
	cipherID := registry.nextID
	registry.nextID++
	registry.ciphers[cipherID] = cipher
	registry.mu.Unlock()

	return js.ValueOf(cipherID)
}

func encryptChunk(_ js.Value, args []js.Value) interface{} {
	if len(args) != 3 {
		return handleError(errors.New("invalid arguments"))
	}

	cipherID := args[0].Int()
	data := make([]byte, args[1].Length())
	js.CopyBytesToGo(data, args[1])
	isLastChunk := args[2].Bool()

	// Get cipher from registry
	registry.mu.Lock()
	cipher, exists := registry.ciphers[cipherID]
	registry.mu.Unlock()

	if !exists {
		return handleError(errors.New("invalid cipher ID"))
	}

	nonce := make([]byte, 12)
	copy(nonce, cipher.iv)
	binary.LittleEndian.PutUint32(nonce[8:], uint32(cipher.chunk))
	cipher.chunk++

	encrypted := cipher.gcm.Seal(nil, nonce, data, nil)

	uint8Array := js.Global().Get("Uint8Array").New(len(encrypted))
	js.CopyBytesToJS(uint8Array, encrypted)

	// If this is the last chunk, clean up the cipher
	if isLastChunk {
		registry.mu.Lock()
		if c, exists := registry.ciphers[cipherID]; exists {
			// Clear the IV for security
			for i := range c.iv {
				c.iv[i] = 0
			}
			delete(registry.ciphers, cipherID)
		}
		registry.mu.Unlock()
	}

	return uint8Array
}

func decryptChunk(_ js.Value, args []js.Value) interface{} {
	if len(args) != 3 {
		return handleError(errors.New("invalid arguments"))
	}

	cipherID := args[0].Int()
	data := make([]byte, args[1].Length())
	js.CopyBytesToGo(data, args[1])
	isLastChunk := args[2].Bool()

	// Get cipher from registry
	registry.mu.Lock()
	cipher, exists := registry.ciphers[cipherID]
	registry.mu.Unlock()

	if !exists {
		return handleError(errors.New("invalid cipher ID"))
	}

	nonce := make([]byte, 12)
	copy(nonce, cipher.iv)
	binary.LittleEndian.PutUint32(nonce[8:], uint32(cipher.chunk))

	decrypted, err := cipher.gcm.Open(nil, nonce, data, nil)
	if err != nil {
		fmt.Printf("Decryption error: %v\n", err)
		return handleError(err)
	}

	cipher.chunk++

	uint8Array := js.Global().Get("Uint8Array").New(len(decrypted))
	js.CopyBytesToJS(uint8Array, decrypted)

	// If this is the last chunk, clean up the cipher
	if isLastChunk {
		registry.mu.Lock()
		if c, exists := registry.ciphers[cipherID]; exists {
			// Clear the IV for security
			for i := range c.iv {
				c.iv[i] = 0
			}
			delete(registry.ciphers, cipherID)
		}
		registry.mu.Unlock()
	}

	return uint8Array
}

// disposeCipher manually cleans up a cipher when no longer needed
func disposeCipher(_ js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		return handleError(errors.New("invalid arguments"))
	}

	cipherID := args[0].Int()

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if cipher, exists := registry.ciphers[cipherID]; exists {
		// Clear the IV for security
		for i := range cipher.iv {
			cipher.iv[i] = 0
		}
		delete(registry.ciphers, cipherID)
	}

	return js.ValueOf(true)
}

// generateKey generates a cryptographically secure random key of a specified size (in bits).
// It supports key sizes of 128, 192, and 256 bits.  It accepts an optional argument
// specifying the key size.  If no argument is provided, it defaults to 128 bits.
// The argument can be either a number or a string that can be parsed as an integer.
// If an invalid key size is provided (either not one of the supported sizes or a non-numeric string),
// it defaults to 128 bits and prints an error message to the console.  The generated key
// is returned as a URL-safe base64 encoded string.
//
// Args:
//
//	_ (js.Value): The "this" value (unused).
//	args ([]js.Value): An array of JavaScript values.  args[0], if present, should be
//	  the desired key size in bits (either as a number or a string).
//
// Returns:
//
//	interface{}: A URL-safe base64 encoded string representing the generated key, or
//	  an error object if key generation fails.
func generateKey(_ js.Value, args []js.Value) interface{} {
	keySizeBits := 128 // Default key size

	// Check if an argument was provided and attempt to parse it.
	if len(args) > 0 {
		if args[0].Type() == js.TypeNumber { //Verify it is a number
			keySizeBits = args[0].Int()
		} else if args[0].Type() == js.TypeString { //Try to parse a string
			parsedSize, err := strconv.Atoi(args[0].String())
			if err != nil {
				fmt.Println("Invalid key size provided, defaulting to 128 bits. Error:", err)
			} else {
				keySizeBits = parsedSize
			}
		} else {
			fmt.Println("Invalid type provided for key size (expected number or string), defaulting to 128 bits.")
		}

	}

	var keySize int
	switch keySizeBits {
	case 128:
		keySize = 16 // 128 bits / 8 bits per byte = 16 bytes
	case 192:
		keySize = 24 // 192 bits / 8 bits per byte = 24 bytes
	case 256:
		keySize = 32 // 256 bits / 8 bits per byte = 32 bytes
	default:
		fmt.Printf("Invalid key size (%d bits), defaulting to 128 bits.\n", keySizeBits)
		keySize = 16 // Default to 128 bits if invalid size provided
	}

	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return handleError(err) // Assuming handleError is defined elsewhere
	}
	return base64.URLEncoding.EncodeToString(key)
}

func decryptMetadata(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	keyBase64 := args[0].String()
	data := make([]byte, args[1].Length())
	js.CopyBytesToGo(data, args[1])

	if len(data) < 16 {
		return handleError(errors.New("invalid metadata format"))
	}

	iv := data[:12]
	metadataLen := binary.LittleEndian.Uint32(data[12:16])
	if len(data) < 16+int(metadataLen) {
		return handleError(errors.New("incomplete metadata"))
	}
	encryptedMetadata := data[16 : 16+metadataLen]

	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return handleError(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return handleError(err)
	}

	decrypted, err := aead.Open(nil, iv, encryptedMetadata, nil)
	if err != nil {
		return handleError(err)
	}

	var metadata Metadata
	if err := json.NewDecoder(bytes.NewReader(decrypted)).Decode(&metadata); err != nil {
		return handleError(err)
	}

	result := make(map[string]interface{})
	result["filename"] = metadata.Filename
	result["contentType"] = metadata.ContentType
	result["size"] = metadata.Size

	return js.ValueOf(result)
}

func handleError(err error) interface{} {
	errorConstructor := js.Global().Get("Error")
	return errorConstructor.New(err.Error())
}
