package decrypt

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRawKeyAndDecryptFile(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, KeySize)
	page1Plain := make([]byte, PageSize)
	copy(page1Plain[:], []byte(sqliteHeader))
	copy(page1Plain[len(sqliteHeader):], bytes.Repeat([]byte{0x11}, PageSize-len(sqliteHeader)-ReserveSize))
	page2Plain := bytes.Repeat([]byte{0x22}, PageSize-ReserveSize)

	page1 := encryptTestPage(t, 1, key, page1Plain[len(sqliteHeader):PageSize-ReserveSize])
	page2 := encryptTestPage(t, 2, key, page2Plain)

	if !ValidateRawKey(page1, key) {
		t.Fatal("expected key to validate")
	}
	wrong := bytes.Repeat([]byte{0x43}, KeySize)
	if ValidateRawKey(page1, wrong) {
		t.Fatal("expected wrong key to fail")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "encrypted.db")
	dst := filepath.Join(dir, "plain.db")
	if err := os.WriteFile(src, append(page1, page2...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DecryptFile(context.Background(), src, dst, hex.EncodeToString(key)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:len(sqliteHeader)], []byte(sqliteHeader)) {
		t.Fatalf("missing sqlite header: %q", got[:len(sqliteHeader)])
	}
	if !bytes.Equal(got[PageSize:PageSize+len(page2Plain)], page2Plain) {
		t.Fatal("page 2 did not decrypt to expected content")
	}
}

func encryptTestPage(t *testing.T, pageNum int, key []byte, plain []byte) []byte {
	t.Helper()
	page := make([]byte, PageSize)
	salt := bytes.Repeat([]byte{0x7a}, SaltSize)
	if pageNum == 1 {
		copy(page[:SaltSize], salt)
	}
	iv := bytes.Repeat([]byte{byte(pageNum)}, IVSize)
	copy(page[PageSize-ReserveSize:PageSize-ReserveSize+IVSize], iv)

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	encrypted := append([]byte(nil), plain...)
	if len(encrypted)%aes.BlockSize != 0 {
		t.Fatalf("plain length %d is not block aligned", len(encrypted))
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, encrypted)
	if pageNum == 1 {
		copy(page[SaltSize:PageSize-ReserveSize], encrypted)
	} else {
		copy(page[:PageSize-ReserveSize], encrypted)
	}

	macKey := deriveRawMACKey(key, salt)
	dataEnd := PageSize - ReserveSize + IVSize
	mac := hmac.New(sha512.New, macKey)
	offset := 0
	if pageNum == 1 {
		offset = SaltSize
	}
	mac.Write(page[offset:dataEnd])
	var pg [4]byte
	binary.LittleEndian.PutUint32(pg[:], uint32(pageNum))
	mac.Write(pg[:])
	copy(page[dataEnd:dataEnd+HMACSize], mac.Sum(nil))
	return page
}
