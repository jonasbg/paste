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
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall/js"

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

	// streamFinalBit marks the final chunk in the STREAM nonce counter.
	// 31-bit counter + 1 final-marker bit gives 2^31 chunks per file.
	streamFinalBit uint32 = 0x80000000
	streamCounterMask uint32 = 0x7FFFFFFF
)

type Metadata struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type StreamingCipher struct {
	gcm      cipher.AEAD
	iv       []byte
	chunk    uint32
	nonce    [12]byte
	plainBuf []byte
	sealBuf  []byte
	openBuf  []byte
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
		"deriveFromPassphrase":   js.FuncOf(deriveFromPassphrase),
	})
	<-c
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// decodeKey decodes a base64 URL-safe key. Accepts both raw (unpadded) and
// padded forms for compatibility with older share URLs.
func decodeKey(s string) ([]byte, error) {
	if k, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return k, nil
	}
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.URLEncoding.DecodeString(s)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, errors.New("invalid key length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func deriveFromPassphrase(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return handleError(errors.New("passphrase required"))
	}
	passphrase := args[0].String()
	keySizeBits := 128
	if len(args) >= 2 && args[1].Type() == js.TypeNumber {
		keySizeBits = args[1].Int()
	}
	var keySize int
	switch keySizeBits {
	case 192:
		keySize = 24
	case 256:
		keySize = 32
	case 128:
		keySize = 16
	default:
		return handleError(errors.New("invalid key size, must be 128, 192, or 256"))
	}

	// Argon2id provides the work factor; HKDF provides labeled domain separation
	// for the file ID and the encryption key.
	stretched := argon2.IDKey([]byte(passphrase), []byte(argon2Salt),
		argon2Time, argon2Memory, argon2Par, argon2Out)
	defer zero(stretched)

	fileIDReader := hkdf.New(sha256.New, stretched, nil, []byte(hkdfFileIDInfo))
	fileIDBytes := make([]byte, 16)
	if _, err := io.ReadFull(fileIDReader, fileIDBytes); err != nil {
		return handleError(err)
	}

	keyReader := hkdf.New(sha256.New, stretched, nil, []byte(hkdfKeyInfo))
	key := make([]byte, keySize)
	if _, err := io.ReadFull(keyReader, key); err != nil {
		return handleError(err)
	}

	fileID := hex(fileIDBytes)
	keyBase64 := base64.RawURLEncoding.EncodeToString(key)
	zero(key)

	return js.ValueOf(map[string]interface{}{
		"fileId": fileID,
		"key":    keyBase64,
	})
}

// hex avoids fmt.Sprintf to keep WASM size down and avoid allocator pressure.
func hex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0f]
	}
	return string(out)
}

func generateHmacToken(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	fileId := args[0].String()
	key, err := decodeKey(args[1].String())
	if err != nil {
		return handleError(err)
	}
	defer zero(key)

	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return handleError(errors.New("invalid key length"))
	}

	hmacKey, err := deriveHMACKey(key, fileId)
	if err != nil {
		return handleError(err)
	}
	defer zero(hmacKey)

	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(fileId))
	signature := h.Sum(nil)

	tokenLength := len(key)
	if tokenLength > len(signature) {
		tokenLength = len(signature)
	}
	token := base64.RawURLEncoding.EncodeToString(signature[:tokenLength])

	const safeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	for _, c := range token {
		if !strings.ContainsRune(safeChars, c) {
			return handleError(errors.New("generated token contains unsafe characters"))
		}
	}

	return js.ValueOf(token)
}

func deriveHMACKey(baseKey []byte, fileID string) ([]byte, error) {
	reader := hkdf.New(sha256.New, baseKey, []byte(fileID), []byte(hkdfHMACInfo))
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

	key, err := decodeKey(args[0].String())
	if err != nil {
		return handleError(err)
	}
	defer zero(key)

	aead, err := newAEAD(key)
	if err != nil {
		return handleError(err)
	}

	data := make([]byte, args[1].Length())
	js.CopyBytesToGo(data, args[1])
	defer zero(data)

	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return handleError(err)
	}

	encrypted := aead.Seal(nil, iv, data, []byte(metadataAAD))
	result := append(iv, encrypted...)

	uint8Array := js.Global().Get("Uint8Array").New(len(result))
	js.CopyBytesToJS(uint8Array, result)
	return uint8Array
}

func createEncryptionStream(_ js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		return handleError(errors.New("invalid arguments"))
	}

	key, err := decodeKey(args[0].String())
	if err != nil {
		return handleError(err)
	}
	defer zero(key)

	aead, err := newAEAD(key)
	if err != nil {
		return handleError(err)
	}

	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return handleError(err)
	}

	sc := &StreamingCipher{gcm: aead, iv: iv}

	registry.mu.Lock()
	cipherID := registry.nextID
	registry.nextID++
	registry.ciphers[cipherID] = sc
	registry.mu.Unlock()

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

	if args[1].Length() != 12 {
		return handleError(errors.New("invalid IV size"))
	}
	iv := make([]byte, 12)
	js.CopyBytesToGo(iv, args[1])

	key, err := decodeKey(args[0].String())
	if err != nil {
		return handleError(err)
	}
	defer zero(key)

	aead, err := newAEAD(key)
	if err != nil {
		return handleError(err)
	}

	sc := &StreamingCipher{gcm: aead, iv: iv}

	registry.mu.Lock()
	cipherID := registry.nextID
	registry.nextID++
	registry.ciphers[cipherID] = sc
	registry.mu.Unlock()

	return js.ValueOf(cipherID)
}

// buildChunkNonce writes the STREAM nonce for chunkIdx into dst.
// dst must be 12 bytes. The high bit of the 32-bit counter encodes isFinal,
// so reordering, truncation, or extension all fail GCM authentication.
func buildChunkNonce(dst []byte, iv []byte, chunkIdx uint32, isFinal bool) {
	copy(dst, iv[:8])
	counter := chunkIdx & streamCounterMask
	if isFinal {
		counter |= streamFinalBit
	}
	binary.LittleEndian.PutUint32(dst[8:], counter)
}

func encryptChunk(_ js.Value, args []js.Value) interface{} {
	if len(args) != 3 {
		return handleError(errors.New("invalid arguments"))
	}

	cipherID := args[0].Int()
	isLast := args[2].Bool()

	registry.mu.Lock()
	sc, exists := registry.ciphers[cipherID]
	registry.mu.Unlock()

	if !exists {
		return handleError(errors.New("invalid cipher ID"))
	}

	if sc.chunk&streamFinalBit != 0 || sc.chunk >= streamCounterMask {
		return handleError(errors.New("chunk counter exhausted"))
	}

	n := args[1].Length()
	if n > len(sc.plainBuf) {
		sc.plainBuf = make([]byte, n)
	}
	data := sc.plainBuf[:n]
	js.CopyBytesToGo(data, args[1])

	buildChunkNonce(sc.nonce[:], sc.iv, sc.chunk, isLast)
	sc.chunk++

	sc.sealBuf = sc.gcm.Seal(sc.sealBuf[:0], sc.nonce[:], data, []byte(chunkAAD))
	zero(data)

	uint8Array := js.Global().Get("Uint8Array").New(len(sc.sealBuf))
	js.CopyBytesToJS(uint8Array, sc.sealBuf)

	if isLast {
		disposeByID(cipherID)
	}

	return uint8Array
}

func decryptChunk(_ js.Value, args []js.Value) interface{} {
	if len(args) != 3 {
		return handleError(errors.New("invalid arguments"))
	}

	cipherID := args[0].Int()
	isLast := args[2].Bool()

	registry.mu.Lock()
	sc, exists := registry.ciphers[cipherID]
	registry.mu.Unlock()

	if !exists {
		return handleError(errors.New("invalid cipher ID"))
	}

	if sc.chunk >= streamCounterMask {
		return handleError(errors.New("chunk counter exhausted"))
	}

	n := args[1].Length()
	if n > len(sc.plainBuf) {
		sc.plainBuf = make([]byte, n)
	}
	data := sc.plainBuf[:n]
	js.CopyBytesToGo(data, args[1])

	buildChunkNonce(sc.nonce[:], sc.iv, sc.chunk, isLast)

	var err error
	sc.openBuf, err = sc.gcm.Open(sc.openBuf[:0], sc.nonce[:], data, []byte(chunkAAD))
	zero(data)
	if err != nil {
		return handleError(err)
	}

	sc.chunk++

	uint8Array := js.Global().Get("Uint8Array").New(len(sc.openBuf))
	js.CopyBytesToJS(uint8Array, sc.openBuf)

	if isLast {
		disposeByID(cipherID)
	}

	return uint8Array
}

func disposeByID(cipherID int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	c, ok := registry.ciphers[cipherID]
	if !ok {
		return
	}
	zero(c.iv)
	zero(c.nonce[:])
	zero(c.plainBuf)
	zero(c.sealBuf)
	zero(c.openBuf)
	delete(registry.ciphers, cipherID)
}

func disposeCipher(_ js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		return handleError(errors.New("invalid arguments"))
	}
	disposeByID(args[0].Int())
	return js.ValueOf(true)
}

func generateKey(_ js.Value, args []js.Value) interface{} {
	keySizeBits := 128

	if len(args) > 0 {
		switch args[0].Type() {
		case js.TypeNumber:
			keySizeBits = args[0].Int()
		case js.TypeString:
			parsed, err := strconv.Atoi(args[0].String())
			if err != nil {
				return handleError(errors.New("invalid key size: not a number"))
			}
			keySizeBits = parsed
		case js.TypeUndefined, js.TypeNull:
			// keep default
		default:
			return handleError(errors.New("invalid key size: must be number or numeric string"))
		}
	}

	var keySize int
	switch keySizeBits {
	case 128:
		keySize = 16
	case 192:
		keySize = 24
	case 256:
		keySize = 32
	default:
		return handleError(errors.New("invalid key size: must be 128, 192, or 256"))
	}

	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return handleError(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	zero(key)
	return encoded
}

func decryptMetadata(_ js.Value, args []js.Value) interface{} {
	if len(args) != 2 {
		return handleError(errors.New("invalid arguments"))
	}

	key, err := decodeKey(args[0].String())
	if err != nil {
		return handleError(err)
	}
	defer zero(key)

	data := make([]byte, args[1].Length())
	js.CopyBytesToGo(data, args[1])

	if len(data) < 16 {
		return handleError(errors.New("invalid metadata format"))
	}

	iv := data[:12]
	metadataLen := binary.LittleEndian.Uint32(data[12:16])
	if uint64(len(data)) < uint64(16)+uint64(metadataLen) {
		return handleError(errors.New("incomplete metadata"))
	}
	encryptedMetadata := data[16 : 16+metadataLen]

	aead, err := newAEAD(key)
	if err != nil {
		return handleError(err)
	}

	decrypted, err := aead.Open(nil, iv, encryptedMetadata, []byte(metadataAAD))
	if err != nil {
		return handleError(err)
	}
	defer zero(decrypted)

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
