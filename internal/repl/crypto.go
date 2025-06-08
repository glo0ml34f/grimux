package repl

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"io"
)

func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	hLen := sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	dk := make([]byte, 0, numBlocks*hLen)
	var blockBuf [4]byte
	for block := 1; block <= numBlocks; block++ {
		binary.BigEndian.PutUint32(blockBuf[:], uint32(block))
		u := hmac.New(sha256.New, password)
		u.Write(salt)
		u.Write(blockBuf[:])
		t := u.Sum(nil)
		out := make([]byte, len(t))
		copy(out, t)
		for i := 1; i < iter; i++ {
			u = hmac.New(sha256.New, password)
			u.Write(t)
			t = u.Sum(nil)
			for j := range t {
				out[j] ^= t[j]
			}
		}
		dk = append(dk, out...)
	}
	return dk[:keyLen]
}

func deriveKey(pass string, salt []byte) []byte {
	return pbkdf2Key([]byte(pass), salt, 100000, 32)
}

func compressData(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressData(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func encryptData(data []byte, pass string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := deriveKey(pass, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, data, nil)
	out := append(salt, append(nonce, ct...)...)
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(out)))
	base64.StdEncoding.Encode(enc, out)
	return enc, nil
}

func decryptData(enc []byte, pass string) ([]byte, error) {
	raw := make([]byte, base64.StdEncoding.DecodedLen(len(enc)))
	n, err := base64.StdEncoding.Decode(raw, enc)
	if err != nil {
		return nil, err
	}
	raw = raw[:n]
	if len(raw) < 16 {
		return nil, io.ErrUnexpectedEOF
	}
	salt := raw[:16]
	key := deriveKey(pass, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < 16+gcm.NonceSize() {
		return nil, io.ErrUnexpectedEOF
	}
	nonce := raw[16 : 16+gcm.NonceSize()]
	ct := raw[16+gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}
