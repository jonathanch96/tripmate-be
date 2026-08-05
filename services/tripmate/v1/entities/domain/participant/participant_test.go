package participant

import "testing"

func TestMaskAccount(t *testing.T) {
	if got := MaskAccount("1234567890"); got != "••••7890" {
		t.Fatalf("got %q", got)
	}
	if got := MaskAccount("123"); got != "••••" {
		t.Fatalf("short=%q", got)
	}
}
