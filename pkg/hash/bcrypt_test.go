package hash

import "testing"

func TestBcryptHashVerifyRoundTrip(t *testing.T) {
	hasher := &BcryptHasher{Cost: 4}
	encoded, err := hasher.Hash("correct horse 1")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := hasher.Verify("correct horse 1", encoded)
	if err != nil || !ok {
		t.Fatalf("verify = %v, %v", ok, err)
	}
	ok, err = hasher.Verify("wrong", encoded)
	if err != nil || ok {
		t.Fatalf("wrong password verify = %v, %v", ok, err)
	}
}

func TestBcryptVerifyFailsClosedOnMalformedOrEmptyHash(t *testing.T) {
	hasher := &BcryptHasher{Cost: 4}
	if ok, err := hasher.Verify("anything", ""); err != nil || ok {
		t.Fatalf("empty hash verify = %v, %v", ok, err)
	}
	if ok, err := hasher.Verify("anything", "not-a-bcrypt-hash"); err != nil || ok {
		t.Fatalf("malformed hash verify = %v, %v", ok, err)
	}
}

func TestBcryptDefaultCost(t *testing.T) {
	if NewBcryptHasher().Cost == 0 {
		t.Fatal("NewBcryptHasher should set a non-zero cost")
	}
}
