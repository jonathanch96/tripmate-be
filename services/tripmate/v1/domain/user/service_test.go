package user

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appjwt "github.com/jblabs/tripmate-be/pkg/jwt"
	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type fakeRepo struct {
	byEmail map[string]*domainuser.User
	created *domainuser.User
}

func (f *fakeRepo) Create(_ context.Context, u *domainuser.User) (*domainuser.User, error) {
	copy := *u
	f.created = &copy
	f.byEmail[u.Email] = &copy
	return &copy, nil
}
func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*domainuser.User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			copy := *u
			return &copy, nil
		}
	}
	return nil, apperror.New("USER_NOT_FOUND")
}
func (f *fakeRepo) GetByEmail(_ context.Context, email string) (*domainuser.User, error) {
	u := f.byEmail[email]
	if u == nil {
		return nil, apperror.New("USER_NOT_FOUND")
	}
	copy := *u
	return &copy, nil
}
func (f *fakeRepo) ExistsByEmail(_ context.Context, email string) (bool, error) {
	return f.byEmail[email] != nil, nil
}
func (f *fakeRepo) Update(_ context.Context, u *domainuser.User) (*domainuser.User, error) {
	copy := *u
	f.byEmail[u.Email] = &copy
	return &copy, nil
}
func (f *fakeRepo) ListByIDs(context.Context, []uuid.UUID) ([]domainuser.User, error) {
	return nil, nil
}

type fakeTokens struct {
	byHash     map[string]*domainuser.RefreshToken
	stored     []domainuser.RefreshToken
	revoked    []uuid.UUID
	revokedAll []uuid.UUID
}

func (f *fakeTokens) Store(_ context.Context, token domainuser.RefreshToken) error {
	copy := token
	f.stored = append(f.stored, copy)
	f.byHash[token.TokenHash] = &copy
	return nil
}
func (f *fakeTokens) GetByHash(_ context.Context, hash string) (*domainuser.RefreshToken, error) {
	return f.byHash[hash], nil
}
func (f *fakeTokens) Revoke(_ context.Context, id uuid.UUID) error {
	f.revoked = append(f.revoked, id)
	return nil
}
func (f *fakeTokens) RevokeAllForUser(_ context.Context, id uuid.UUID) error {
	f.revokedAll = append(f.revokedAll, id)
	return nil
}

type fakeHasher struct{ verifyCalls int }

func (f *fakeHasher) Hash(string) (string, error) { return "hashed", nil }
func (f *fakeHasher) Verify(plain, hash string) (bool, error) {
	f.verifyCalls++
	return plain == "Password1" && hash == "hashed", nil
}

type fakeIssuer struct {
	sequence int
	now      time.Time
}

func (f *fakeIssuer) IssueAccess(domainuser.User) (string, time.Time, error) {
	f.sequence++
	return "access", f.now.Add(15 * time.Minute), nil
}
func (f *fakeIssuer) IssueRefresh(domainuser.User) (string, time.Time, error) {
	return "refresh-" + string(rune('0'+f.sequence)), f.now.Add(30 * 24 * time.Hour), nil
}
func (f *fakeIssuer) ParseAccess(string) (*appjwt.Claims, error) { return nil, nil }

func fixture() (*service, *fakeRepo, *fakeTokens, *fakeHasher) {
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	repo := &fakeRepo{byEmail: map[string]*domainuser.User{}}
	tokens := &fakeTokens{byHash: map[string]*domainuser.RefreshToken{}}
	hasher := &fakeHasher{}
	return NewService(Dependencies{Repo: repo, Tokens: tokens, Hasher: hasher, Issuer: &fakeIssuer{now: now}, Clock: func() time.Time { return now }}).(*service), repo, tokens, hasher
}

func TestRegisterNormalizesEmailHashesPasswordAndStoresOnlyRefreshHash(t *testing.T) {
	service, repo, tokens, _ := fixture()
	session, err := service.Register(context.Background(), RegisterInput{Email: " Person@Example.COM ", Name: " Person ", Password: "Password1"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.created.Email != "person@example.com" || repo.created.Name != "Person" || repo.created.PasswordHash != "hashed" {
		t.Fatalf("created = %+v", repo.created)
	}
	if len(tokens.stored) != 1 || tokens.stored[0].TokenHash == session.RefreshToken || len(tokens.stored[0].TokenHash) != 64 {
		t.Fatalf("stored raw refresh token: %+v", tokens.stored)
	}
}

type pendingInvitationFinderStub struct {
	email string
	rows  []domaininvitation.Invitation
}

func (f *pendingInvitationFinderStub) ListPendingByEmail(_ context.Context, email string) ([]domaininvitation.Invitation, error) {
	f.email = email
	return f.rows, nil
}

func TestRegisterSurfacesPendingInvitationsWithoutAcceptingThem(t *testing.T) {
	service, _, _, _ := fixture()
	pending := domaininvitation.Invitation{ID: uuid.New(), Email: "person@example.com", Status: domaininvitation.StatusPending}
	finder := &pendingInvitationFinderStub{rows: []domaininvitation.Invitation{pending}}
	service.deps.Invitations = finder

	session, err := service.Register(context.Background(), RegisterInput{
		Email: " Person@Example.COM ", Name: "Person", Password: "Password1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if finder.email != "person@example.com" || len(session.PendingInvitations) != 1 {
		t.Fatalf("pending invitations = %+v, lookup email = %q", session.PendingInvitations, finder.email)
	}
	if session.PendingInvitations[0].Status != domaininvitation.StatusPending || session.PendingInvitations[0].AcceptedAt != nil {
		t.Fatalf("registration changed invitation state: %+v", session.PendingInvitations[0])
	}
}

func TestRegisterRejectsDuplicateAndWeakPassword(t *testing.T) {
	service, repo, _, _ := fixture()
	repo.byEmail["used@example.com"] = &domainuser.User{ID: uuid.New(), Email: "used@example.com"}
	if _, err := service.Register(context.Background(), RegisterInput{Email: "used@example.com", Name: "A", Password: "Password1"}); !apperror.Is(err, "EMAIL_ALREADY_REGISTERED") {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := service.Register(context.Background(), RegisterInput{Email: "new@example.com", Name: "A", Password: "letters"}); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("weak = %v", err)
	}
}

func TestAuthenticateUnknownEmailAndWrongPasswordAreIndistinguishable(t *testing.T) {
	service, repo, _, hasher := fixture()
	repo.byEmail["known@example.com"] = &domainuser.User{ID: uuid.New(), Email: "known@example.com", PasswordHash: "hashed"}
	start := time.Now()
	_, unknown := service.Authenticate(context.Background(), "missing@example.com", "wrong")
	unknownDuration := time.Since(start)
	if hasher.verifyCalls != 1 || !apperror.Is(unknown, "INVALID_CREDENTIALS") {
		t.Fatalf("unknown = %v calls=%d", unknown, hasher.verifyCalls)
	}
	start = time.Now()
	_, wrong := service.Authenticate(context.Background(), "known@example.com", "wrong")
	wrongDuration := time.Since(start)
	if hasher.verifyCalls != 2 || !apperror.Is(wrong, "INVALID_CREDENTIALS") {
		t.Fatalf("wrong = %v calls=%d", wrong, hasher.verifyCalls)
	}
	delta := unknownDuration - wrongDuration
	if delta < 0 {
		delta = -delta
	}
	if delta > 20*time.Millisecond {
		t.Fatalf("timing delta = %v", delta)
	}
}

func TestAuthenticateSuccess(t *testing.T) {
	service, repo, tokens, _ := fixture()
	repo.byEmail["known@example.com"] = &domainuser.User{ID: uuid.New(), Email: "known@example.com", PasswordHash: "hashed"}
	if _, err := service.Authenticate(context.Background(), " KNOWN@example.com ", "Password1"); err != nil || len(tokens.stored) != 1 {
		t.Fatalf("authenticate = %v", err)
	}
}

func TestRefreshRotatesPresentedToken(t *testing.T) {
	service, repo, tokens, _ := fixture()
	userID, tokenID := uuid.New(), uuid.New()
	repo.byEmail["a@example.com"] = &domainuser.User{ID: userID, Email: "a@example.com"}
	tokens.byHash[tokenHash("old")] = &domainuser.RefreshToken{ID: tokenID, UserID: userID, ExpiresAt: service.deps.Clock().Add(time.Hour)}
	if _, err := service.Refresh(context.Background(), "old"); err != nil {
		t.Fatal(err)
	}
	if len(tokens.revoked) != 1 || tokens.revoked[0] != tokenID || len(tokens.stored) != 1 {
		t.Fatalf("rotation failed: %+v", tokens)
	}
}

func TestRefreshReuseRevokesAllForUser(t *testing.T) {
	service, _, tokens, _ := fixture()
	now, userID := service.deps.Clock(), uuid.New()
	tokens.byHash[tokenHash("used")] = &domainuser.RefreshToken{ID: uuid.New(), UserID: userID, ExpiresAt: now.Add(time.Hour), RevokedAt: &now}
	if _, err := service.Refresh(context.Background(), "used"); !apperror.Is(err, "UNAUTHENTICATED") {
		t.Fatalf("reuse = %v", err)
	}
	if len(tokens.revokedAll) != 1 || tokens.revokedAll[0] != userID {
		t.Fatalf("cascade = %+v", tokens.revokedAll)
	}
}

func TestLogoutUnknownIsSilentAndKnownIsRevoked(t *testing.T) {
	service, _, tokens, _ := fixture()
	if err := service.Logout(context.Background(), "unknown"); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	tokens.byHash[tokenHash("known")] = &domainuser.RefreshToken{ID: id}
	if err := service.Logout(context.Background(), "known"); err != nil || len(tokens.revoked) != 1 {
		t.Fatalf("logout = %v", err)
	}
}
