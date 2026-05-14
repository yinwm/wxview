package app

import "testing"

func TestSudoOwnerFromEnv(t *testing.T) {
	uid, gid, ok := sudoOwnerFromEnv(0, "501", "20")
	if !ok {
		t.Fatal("expected sudo owner")
	}
	if uid != 501 || gid != 20 {
		t.Fatalf("uid/gid = %d/%d", uid, gid)
	}

	if _, _, ok := sudoOwnerFromEnv(501, "501", "20"); ok {
		t.Fatal("non-root process should not use sudo owner")
	}
	if _, _, ok := sudoOwnerFromEnv(0, "root", "20"); ok {
		t.Fatal("invalid uid should not be accepted")
	}
}

func TestSafeAccountDirKeepsReadableWeChatID(t *testing.T) {
	got := SafeAccountDir("wxid_c7zf4hpjug5322_bcc2")
	if got != "wxid_c7zf4hpjug5322_bcc2" {
		t.Fatalf("SafeAccountDir = %q", got)
	}
}

func TestSafeAccountDirSanitizesPathSeparators(t *testing.T) {
	got := SafeAccountDir("a/b c")
	if got != "a_b_c" {
		t.Fatalf("SafeAccountDir = %q", got)
	}
}
