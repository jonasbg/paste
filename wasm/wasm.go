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
	"strconv"
	"strings"
	"syscall/js"
)

const (
	chunkSize = 1 * 1024 * 1024 // 1MB chunks
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

var activeCipher *StreamingCipher

func main() {
	c := make(chan struct{}, 0)
	js.Global().Set("goEncryption", map[string]interface{}{
		"createEncryptionStream": js.FuncOf(createEncryptionStream),
		"createDecryptionStream": js.FuncOf(createDecryptionStream),
		"encryptChunk":           js.FuncOf(encryptChunk),
		"decryptChunk":           js.FuncOf(decryptChunk),
		"generateKey":            js.FuncOf(generateKey),
		"decryptMetadata":        js.FuncOf(decryptMetadata),
		"encrypt":                js.FuncOf(encrypt), // Keep the original encrypt
		"decrypt":                js.FuncOf(decrypt), // Keep the original decrypt
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

	// Decode the base64 key
	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	// Create HMAC
	h := hmac.New(sha256.New, key)
	h.Write([]byte(fileId))
	signature := h.Sum(nil)

	// Take first 12 bytes (will give us exactly 16 base64 chars)
	token := base64.URLEncoding.EncodeToString(signature[:12])

	// Do base64url transformations
	token = strings.ReplaceAll(token, "+", "-")
	token = strings.ReplaceAll(token, "/", "_")
	token = strings.TrimRight(token, "=")

	// Validate the token is filename safe
	safeChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	for _, char := range token {
		if !strings.ContainsRune(safeChars, char) {
			return handleError(errors.New("generated token contains unsafe characters"))
		}
	}

	return js.ValueOf(token)
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

func decrypt(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	keyBase64 := args[0].String()
	encrypted := make([]byte, args[1].Length())
	js.CopyBytesToGo(encrypted, args[1])

	if len(encrypted) < 12 {
		return handleError(errors.New("invalid encrypted data: no IV"))
	}

	key, err := base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return handleError(err)
	}

	iv := encrypted[:12]
	ciphertext := encrypted[12:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return handleError(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return handleError(err)
	}

	decrypted, err := aead.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return handleError(err)
	}

	uint8Array := js.Global().Get("Uint8Array").New(len(decrypted))
	js.CopyBytesToJS(uint8Array, decrypted)
	return uint8Array
}

func createEncryptionStream(_ js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		return handleError(errors.New("invalid arguments"))
	}

	// Clean up any existing cipher before creating a new one
	if activeCipher != nil {
		// Clear the IV for security
		for i := range activeCipher.iv {
			activeCipher.iv[i] = 0
		}
		// Set to nil to help Go's GC
		activeCipher = nil
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

	activeCipher = &StreamingCipher{
		gcm:   aead,
		iv:    iv,
		chunk: 0,
	}

	uint8Array := js.Global().Get("Uint8Array").New(len(iv))
	js.CopyBytesToJS(uint8Array, iv)
	return uint8Array
}

func createDecryptionStream(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	// Clean up any existing cipher before creating a new one
	if activeCipher != nil {
		// Clear the IV for security
		for i := range activeCipher.iv {
			activeCipher.iv[i] = 0
		}
		// Set to nil to help Go's GC
		activeCipher = nil
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

	activeCipher = &StreamingCipher{
		gcm:   aead,
		iv:    iv,
		chunk: 0,
	}

	return js.ValueOf(true)
}

func encryptChunk(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	if activeCipher == nil {
		return handleError(errors.New("no active encryption stream"))
	}

	data := make([]byte, args[0].Length())
	js.CopyBytesToGo(data, args[0])
	isLastChunk := args[1].Bool()

	nonce := make([]byte, 12)
	copy(nonce, activeCipher.iv)
	binary.LittleEndian.PutUint32(nonce[8:], uint32(activeCipher.chunk))
	activeCipher.chunk++

	encrypted := activeCipher.gcm.Seal(nil, nonce, data, nil)

	uint8Array := js.Global().Get("Uint8Array").New(len(encrypted))
	js.CopyBytesToJS(uint8Array, encrypted)

	// If this is the last chunk, fully clean up the cipher
	if isLastChunk {
		// Clear the IV for security
		for i := range activeCipher.iv {
			activeCipher.iv[i] = 0
		}
		// Set to nil to help Go's GC
		activeCipher = nil
	}

	return uint8Array
}

func decryptChunk(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	if activeCipher == nil {
		return handleError(errors.New("no active decryption stream"))
	}

	data := make([]byte, args[0].Length())
	js.CopyBytesToGo(data, args[0])
	isLastChunk := args[1].Bool()

	nonce := make([]byte, 12)
	copy(nonce, activeCipher.iv)
	binary.LittleEndian.PutUint32(nonce[8:], uint32(activeCipher.chunk))

	decrypted, err := activeCipher.gcm.Open(nil, nonce, data, nil)
	if err != nil {
		fmt.Printf("Decryption error: %v\n", err)
		return handleError(err)
	}

	activeCipher.chunk++

	uint8Array := js.Global().Get("Uint8Array").New(len(decrypted))
	js.CopyBytesToJS(uint8Array, decrypted)

	// If this is the last chunk, fully clean up the cipher
	if isLastChunk {
		// Clear the IV for security
		for i := range activeCipher.iv {
			activeCipher.iv[i] = 0
		}
		// Set to nil to help Go's GC
		activeCipher = nil
	}

	return uint8Array
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
