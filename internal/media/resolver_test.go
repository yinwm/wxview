package media

import (
	"bytes"
	"crypto/aes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveImagePathMacOS4ExactThumbnail(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "wxid_owner_bcc2", "db_storage")
	chat := "53740852064@chatroom"
	imageDir := filepath.Join(dir, "wxid_owner_bcc2", "Message", "MessageTemp", md5Hex(chat), "Image")
	if err := os.MkdirAll(imageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(imageDir, "361778758602_.pic_thumb.jpg")
	if err := os.WriteFile(want, []byte{0xff, 0xd8, 0xff, 0x00}, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := resolveImagePath(dataDir, "", 1778758602, chat, 36)
	if !ok {
		t.Fatal("expected image path")
	}
	if got.path != want || !got.thumbnail {
		t.Fatalf("candidate = %+v, want %s thumbnail", got, want)
	}
}

func TestDecryptXORDatImage(t *testing.T) {
	dir := t.TempDir()
	raw := []byte{0xff, 0xd8, 0xff, 'j', 'p', 'g'}
	path := filepath.Join(dir, "a.dat")
	if err := os.WriteFile(path, xorBytes(raw, 0x55), 0o600); err != nil {
		t.Fatal(err)
	}

	decoded, ext, _, err := decryptImageBytes(path, imageKeyMaterial{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) || ext != "jpg" {
		t.Fatalf("decoded=%x ext=%s", decoded, ext)
	}
}

func TestDecryptWechatV2DatImage(t *testing.T) {
	code := uint32(123456)
	wxid := "wxid_test_a0e4"
	aesKey := deriveV2AESKey(code, wxid)
	raw := []byte{0xff, 0xd8, 0xff, 'v', '2', '-', 'i', 'm', 'g'}
	padded := append([]byte{}, raw...)
	pad := aes.BlockSize - (len(padded) % aes.BlockSize)
	padded = append(padded, bytes.Repeat([]byte{byte(pad)}, pad)...)

	block, err := aes.NewCipher([]byte(aesKey))
	if err != nil {
		t.Fatal(err)
	}
	encrypted := append([]byte{}, padded...)
	for i := 0; i < len(encrypted); i += aes.BlockSize {
		block.Encrypt(encrypted[i:i+aes.BlockSize], encrypted[i:i+aes.BlockSize])
	}

	header := make([]byte, wechatV2HeaderSize)
	copy(header, wechatV2Magic)
	writeI32LEForTest(header, 6, len(raw))
	writeI32LEForTest(header, 10, 0)
	data := append(header, encrypted...)

	dir := t.TempDir()
	path := filepath.Join(dir, wxid, "Image", "a.dat")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	decoded, ext, _, err := decryptImageBytes(path, imageKeyMaterial{KVCommCodes: []uint32{code}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) || ext != "jpg" {
		t.Fatalf("decoded=%x ext=%s", decoded, ext)
	}
}

func TestRejectsWeakBMPHeaderFalsePositive(t *testing.T) {
	data := []byte{
		0x42, 0x4d, 0xf7, 0xfb, 0x6e, 0x34, 0xf2, 0x64,
		0xa9, 0x63, 0xa8, 0x68, 0x3c, 0x82, 0x56, 0xcf,
		0x60, 0xe5, 0x5f, 0x89, 0xb7, 0x77, 0x94, 0x57,
		0x14, 0xfa, 0xcb, 0x4b, 0x65, 0x8c, 0x61, 0xe7,
	}
	if ext := detectImageExtension(data); ext != "" {
		t.Fatalf("weak BMP-like header detected as %s", ext)
	}
}

func TestWXIDCandidatesPreferAccountThenStrippedSuffix(t *testing.T) {
	got := collectWXIDCandidates("/tmp/xwechat_files/wxid_owner_bcc2/msg/attach/a.dat")
	want := []string{"wxid_owner_bcc2", "wxid_owner", "wxid"}
	for i, value := range want {
		if i >= len(got) || got[i] != value {
			t.Fatalf("wxid candidates = %#v, want prefix %#v", got, want)
		}
	}
}

func TestResolverAddsDirectImagePath(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "wxid_owner_bcc2", "db_storage")
	chat := "alice"
	imageDir := filepath.Join(dir, "wxid_owner_bcc2", "Message", "MessageTemp", md5Hex(chat), "Image")
	if err := os.MkdirAll(imageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(imageDir, "71700000000_.pic_thumb.jpg")
	writeTestPNG(t, imagePath, 2, 3)

	resolver := NewResolver(dataDir, filepath.Join(dir, "cache"))
	info := resolver.ResolveImage(chat, 7, 1700000000, 3, "<msg><img /></msg>", false)
	if info.Status != "resolved" || info.Path != imagePath || info.Decoded || !info.Thumbnail {
		t.Fatalf("unexpected media info: %+v", info)
	}
	if info.Width != 2 || info.Height != 3 {
		t.Fatalf("dimensions = %dx%d, want 2x3", info.Width, info.Height)
	}
}

func writeI32LEForTest(data []byte, offset int, value int) {
	data[offset] = byte(value)
	data[offset+1] = byte(value >> 8)
	data[offset+2] = byte(value >> 16)
	data[offset+3] = byte(value >> 24)
}

func writeTestPNG(t *testing.T, path string, width int, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0x44, G: 0x88, B: 0xcc, A: 0xff})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}
