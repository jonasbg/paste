package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"testing"
)

func TestMetadataRoundtrip(t *testing.T) {
	key, err := GenerateKey(16)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte(`{"filename":"hello.txt","size":42}`)
	enc, err := EncryptMetadata(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptMetadata(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, plaintext)
	}
}

func TestMetadataAADIsolation(t *testing.T) {
	// A blob sealed with a v1-style nil AAD must not decrypt under v2.
	// We synthesise a v1 blob by sealing with empty AAD directly.
	key, err := GenerateKey(16)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := newAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("hello")
	sealed := aead.Seal(nil, iv, plaintext, nil) // v1 style: nil AAD

	header := make([]byte, 16)
	copy(header[:12], iv)
	header[12] = byte(len(sealed))
	v1Blob := append(header, sealed...)

	if _, err := DecryptMetadata(key, v1Blob); err == nil {
		t.Fatal("v1 metadata blob unexpectedly decrypted under v2")
	}
}

func TestStreamRoundtripExactMultiple(t *testing.T) {
	// Regression: file size that is an exact multiple of the chunk size.
	// The final chunk is full-size and must still be marked as final.
	testStreamRoundtrip(t, 3, 1024, 1024)
}

func TestStreamRoundtripPartialFinal(t *testing.T) {
	testStreamRoundtrip(t, 3, 1024, 600)
}

func TestStreamRoundtripSingleChunk(t *testing.T) {
	testStreamRoundtrip(t, 1, 1024, 1)
}

func testStreamRoundtrip(t *testing.T, fullChunks, chunkSize, lastSize int) {
	t.Helper()
	key, err := GenerateKey(32)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := make([]byte, fullChunks*chunkSize+lastSize)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatal(err)
	}

	enc, err := NewStreamCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	iv := append([]byte(nil), enc.IV()...)

	var ciphertexts [][]byte
	pieces := splitFixed(plaintext, chunkSize)
	for i, p := range pieces {
		ct, err := enc.EncryptChunk(p, i == len(pieces)-1)
		if err != nil {
			t.Fatal(err)
		}
		ciphertexts = append(ciphertexts, ct)
	}

	dec, err := NewStreamDecryptor(key, iv)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for i, ct := range ciphertexts {
		pt, err := dec.DecryptChunk(ct, i == len(ciphertexts)-1)
		if err != nil {
			t.Fatalf("decrypt chunk %d: %v", i, err)
		}
		out.Write(pt)
	}
	if !bytes.Equal(out.Bytes(), plaintext) {
		t.Fatalf("plaintext mismatch: got %d bytes, want %d", out.Len(), len(plaintext))
	}
}

func splitFixed(data []byte, chunk int) [][]byte {
	var out [][]byte
	for len(data) > chunk {
		out = append(out, data[:chunk])
		data = data[chunk:]
	}
	if len(data) > 0 {
		out = append(out, data)
	}
	return out
}

func TestStreamRejectsTruncation(t *testing.T) {
	// Drop the final chunk. The receiver, having processed fewer chunks than
	// the sender, never sees a chunk with isFinal=true. With the v2 STREAM
	// nonce, any attempt to mark a non-final chunk as final fails.
	key, _ := GenerateKey(16)
	plaintext := make([]byte, 3000)
	rand.Read(plaintext)

	enc, _ := NewStreamCipher(key)
	iv := append([]byte(nil), enc.IV()...)
	c1, _ := enc.EncryptChunk(plaintext[:1000], false)
	c2, _ := enc.EncryptChunk(plaintext[1000:2000], false)
	_, _ = enc.EncryptChunk(plaintext[2000:], true) // discarded by attacker

	dec, _ := NewStreamDecryptor(key, iv)
	if _, err := dec.DecryptChunk(c1, false); err != nil {
		t.Fatal(err)
	}
	// Attacker presents c2 as the final chunk to make the truncation invisible.
	if _, err := dec.DecryptChunk(c2, true); err == nil {
		t.Fatal("truncation succeeded: c2 decrypted as final but was sealed as non-final")
	}
}

func TestStreamRejectsReorder(t *testing.T) {
	key, _ := GenerateKey(16)
	plaintext := make([]byte, 3000)
	rand.Read(plaintext)

	enc, _ := NewStreamCipher(key)
	iv := append([]byte(nil), enc.IV()...)
	_, _ = enc.EncryptChunk(plaintext[:1000], false)
	c2, _ := enc.EncryptChunk(plaintext[1000:2000], false)
	_, _ = enc.EncryptChunk(plaintext[2000:], true)

	dec, _ := NewStreamDecryptor(key, iv)
	// Present c2 in position 0 — should fail GCM auth (counter mismatch).
	if _, err := dec.DecryptChunk(c2, false); err == nil {
		t.Fatal("reorder succeeded: c2 decrypted at position 0")
	}
}

func TestStreamRejectsIsFinalMismatch(t *testing.T) {
	key, _ := GenerateKey(16)
	plaintext := []byte("only chunk")

	enc, _ := NewStreamCipher(key)
	iv := append([]byte(nil), enc.IV()...)
	ct, _ := enc.EncryptChunk(plaintext, true)

	dec, _ := NewStreamDecryptor(key, iv)
	if _, err := dec.DecryptChunk(ct, false); err == nil {
		t.Fatal("isFinal mismatch (sealed true, opened false) was not detected")
	}
}

func TestHMACTokenRoundtrip(t *testing.T) {
	key, _ := GenerateKey(16)
	fileID := "0123456789abcdef0123456789abcdef"
	tok1, err := GenerateHMACToken(fileID, key)
	if err != nil {
		t.Fatal(err)
	}
	tok2, err := GenerateHMACToken(fileID, key)
	if err != nil {
		t.Fatal(err)
	}
	if tok1 != tok2 {
		t.Fatalf("HMAC token not deterministic: %q vs %q", tok1, tok2)
	}
	// Different fileID must yield different token.
	tok3, _ := GenerateHMACToken("ffffffffffffffffffffffffffffffff", key)
	if tok1 == tok3 {
		t.Fatal("HMAC token did not vary with fileID")
	}
}

func TestPassphraseDerivationDeterministic(t *testing.T) {
	// Argon2id is expensive — keep the test small. Two calls with the same
	// input must produce identical output (deterministic), and varying the
	// input must yield different output (no salt-only mode bug).
	pass := "able-acid-aged-also-x7k3"
	id1, k1, err := DeriveFromPassphrase(pass, 16)
	if err != nil {
		t.Fatal(err)
	}
	id2, k2, err := DeriveFromPassphrase(pass, 16)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 || !bytes.Equal(k1, k2) {
		t.Fatal("passphrase derivation not deterministic")
	}
	id3, k3, err := DeriveFromPassphrase(pass+"-extra", 16)
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id3 || bytes.Equal(k1, k3) {
		t.Fatal("different passphrases yielded same key/id")
	}
	if len(id1) != 32 {
		t.Fatalf("fileID length: got %d, want 32 hex chars", len(id1))
	}
}

func TestPassphraseFileIDIsNotJustHash(t *testing.T) {
	// Sanity: the file ID derived from a passphrase should not equal the
	// passphrase under any trivial encoding — guards against a regression
	// where the derivation accidentally returns the passphrase bytes.
	pass := "test-passphrase-value-1234"
	id, _, _ := DeriveFromPassphrase(pass, 16)
	if id == pass {
		t.Fatal("fileID equals passphrase")
	}
	if j, err := json.Marshal(id); err == nil && bytes.Contains(j, []byte(pass)) {
		t.Fatal("fileID contains passphrase bytes")
	}
}
