package academic

import (
	"strings"
	"testing"
)

func TestValidStudentQR(t *testing.T) {
	valid := "SGQR1:" + strings.Repeat("ab", 32)
	if !validStudentQR(valid) {
		t.Fatal("valid token rejected")
	}
	for _, s := range []string{"", valid + "a", strings.TrimPrefix(valid, "SGQR1:"), "SGQR1:" + strings.Repeat("z", 64), "https://example.com/" + valid} {
		if validStudentQR(s) {
			t.Fatal("invalid token accepted")
		}
	}
}
