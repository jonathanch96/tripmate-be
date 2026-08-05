package apperror

import (
	"errors"
	"testing"
)

func TestCatalogComplete(t *testing.T) {
	t.Parallel()
	if len(catalog) != 30 {
		t.Fatalf("catalog has %d entries, want 30", len(catalog))
	}
	for code, item := range catalog {
		if code == "" || item.HTTP == 0 || item.Message == "" {
			t.Errorf("incomplete catalog entry %q: %+v", code, item)
		}
	}
}

func TestWrapAndIs(t *testing.T) {
	t.Parallel()
	cause := errors.New("database detail")
	err := Wrap(cause, "USER_NOT_FOUND")
	if !errors.Is(err, cause) || !Is(err, "USER_NOT_FOUND") {
		t.Fatalf("error chain was not preserved: %v", err)
	}
}

func TestUnknownCodeBecomesInternalError(t *testing.T) {
	t.Parallel()
	err := New("DOES_NOT_EXIST")
	if err.Code != "INTERNAL_ERROR" || err.HTTP != 500 {
		t.Fatalf("unexpected fallback: %+v", err)
	}
}
