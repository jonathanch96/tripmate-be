package trip

import (
	"strings"
	"testing"
)

func TestCryptoCodeGeneratorCharset(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := (CryptoCodeGenerator{}).Generate()
		if err != nil || len(code) != 6 {
			t.Fatalf("code=%q err=%v", code, err)
		}
		for _, r := range code {
			if !strings.ContainsRune(codeAlphabet, r) {
				t.Fatalf("invalid code %q", code)
			}
		}
	}
}
