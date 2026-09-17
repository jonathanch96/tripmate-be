package hash

import "golang.org/x/crypto/bcrypt"

// BcryptHasher backs the operator master-password login only - regular account passwords use
// Argon2Hasher. Bcrypt is used here instead because its hash format is what most standalone
// "generate a password hash" tooling (htpasswd, the bcrypt CLI, online generators) produces, so an
// operator can create MASTER_PASSWORD_HASH without a tripmate-be checkout.
type BcryptHasher struct{ Cost int }

func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{Cost: bcrypt.DefaultCost}
}

func (h *BcryptHasher) Hash(plain string) (string, error) {
	cost := h.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	encoded, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	return string(encoded), err
}

// Verify reports whether plain matches encoded. A malformed encoded value (including empty
// string) is treated as "no match" rather than an error, so a missing/misconfigured
// MASTER_PASSWORD_HASH fails closed instead of surfacing a 500.
func (h *BcryptHasher) Verify(plain, encoded string) (bool, error) {
	if encoded == "" {
		return false, nil
	}
	err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(plain))
	if err != nil {
		return false, nil
	}
	return true, nil
}
