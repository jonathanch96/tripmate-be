package userresponse

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

func TestResponseNeverContainsPasswordHash(t *testing.T) {
	encoded, err := json.Marshal(FromDomain(domainuser.User{ID: uuid.New(), Email: "a@example.com", Name: "A", PasswordHash: "super-secret-hash"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret-hash") || strings.Contains(string(encoded), "hash") {
		t.Fatalf("leaked response: %s", encoded)
	}
	// has_password is the one password-related field that may appear: a bare bool saying whether
	// a password exists, which the account screen needs to know whether to ask for the current
	// one. Every key is checked so a future field carrying the hash itself cannot slip in.
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for key, value := range fields {
		if !strings.Contains(key, "password") {
			continue
		}
		if key != "has_password" {
			t.Fatalf("unexpected password field %q in response: %s", key, encoded)
		}
		if _, ok := value.(bool); !ok {
			t.Fatalf("has_password = %v (%T), want a bool", value, value)
		}
	}
}

func TestHasPasswordTracksWhetherOneIsSet(t *testing.T) {
	googleID := "google-sub"
	withPassword := FromDomain(domainuser.User{ID: uuid.New(), Email: "a@example.com", Name: "A", PasswordHash: "hashed"})
	googleOnly := FromDomain(domainuser.User{ID: uuid.New(), Email: "b@example.com", Name: "B", GoogleID: &googleID})

	if !withPassword.HasPassword {
		t.Fatal("HasPassword = false for an account with a password hash")
	}
	// A Google-only account has credentials but no password, so it is asked for none.
	if googleOnly.HasPassword || !googleOnly.HasAccount {
		t.Fatalf("google-only account: HasPassword = %v, HasAccount = %v", googleOnly.HasPassword, googleOnly.HasAccount)
	}
}
