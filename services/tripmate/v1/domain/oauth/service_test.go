package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type fakeRepo struct {
	clients  map[string]*domainoauth.Client
	requests map[uuid.UUID]*domainoauth.AuthRequest
	grants   map[uuid.UUID]*domainoauth.Grant
	tokens   map[string]*domainoauth.Token
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{clients: map[string]*domainoauth.Client{}, requests: map[uuid.UUID]*domainoauth.AuthRequest{},
		grants: map[uuid.UUID]*domainoauth.Grant{}, tokens: map[string]*domainoauth.Token{}}
}

func (r *fakeRepo) clientByID(id uuid.UUID) *domainoauth.Client {
	for _, client := range r.clients {
		if client.ID == id {
			copy := *client
			return &copy
		}
	}
	return nil
}

func (r *fakeRepo) CreateClient(_ context.Context, c *domainoauth.Client) (*domainoauth.Client, error) {
	copy := *c
	r.clients[c.ClientID] = &copy
	return c, nil
}
func (r *fakeRepo) GetClientByClientID(_ context.Context, id string) (*domainoauth.Client, error) {
	if c, ok := r.clients[id]; ok {
		copy := *c
		return &copy, nil
	}
	return nil, apperror.New("OAUTH_INVALID_CLIENT")
}
func (r *fakeRepo) CreateRequest(_ context.Context, q *domainoauth.AuthRequest) (*domainoauth.AuthRequest, error) {
	copy := *q
	r.requests[q.ID] = &copy
	return q, nil
}
func (r *fakeRepo) GetRequest(_ context.Context, id uuid.UUID) (*domainoauth.AuthRequest, error) {
	q, ok := r.requests[id]
	if !ok {
		return nil, apperror.New("OAUTH_REQUEST_NOT_FOUND")
	}
	copy := *q
	copy.Client = r.clientByID(q.ClientID)
	return &copy, nil
}
func (r *fakeRepo) GetRequestByCodeHash(_ context.Context, hash string) (*domainoauth.AuthRequest, error) {
	for _, q := range r.requests {
		if q.CodeHash != nil && *q.CodeHash == hash {
			copy := *q
			return &copy, nil
		}
	}
	return nil, apperror.New("OAUTH_INVALID_GRANT")
}
func (r *fakeRepo) UpdateRequestStatus(_ context.Context, q *domainoauth.AuthRequest, from domainoauth.RequestStatus) (bool, error) {
	stored := r.requests[q.ID]
	if stored == nil || stored.Status != from {
		return false, nil
	}
	stored.Status, stored.UserID, stored.GrantedScopes, stored.CodeHash, stored.CodeExpiresAt = q.Status, q.UserID, q.GrantedScopes, q.CodeHash, q.CodeExpiresAt
	return true, nil
}
func (r *fakeRepo) GetActiveGrant(_ context.Context, userID, clientID uuid.UUID) (*domainoauth.Grant, error) {
	for _, g := range r.grants {
		if g.UserID == userID && g.ClientID == clientID && g.RevokedAt == nil {
			copy := *g
			return &copy, nil
		}
	}
	return nil, apperror.New("OAUTH_GRANT_NOT_FOUND")
}
func (r *fakeRepo) GetGrant(_ context.Context, id uuid.UUID) (*domainoauth.Grant, error) {
	g, ok := r.grants[id]
	if !ok {
		return nil, apperror.New("OAUTH_GRANT_NOT_FOUND")
	}
	copy := *g
	copy.Client = r.clientByID(g.ClientID)
	return &copy, nil
}
func (r *fakeRepo) CreateGrant(_ context.Context, g *domainoauth.Grant) (*domainoauth.Grant, error) {
	copy := *g
	r.grants[g.ID] = &copy
	return g, nil
}
func (r *fakeRepo) UpdateGrantScopes(_ context.Context, id uuid.UUID, scopes []string) error {
	r.grants[id].Scopes = scopes
	return nil
}
func (r *fakeRepo) TouchGrant(_ context.Context, id uuid.UUID, at time.Time) error {
	r.grants[id].LastUsedAt = at
	return nil
}
func (r *fakeRepo) RevokeGrant(_ context.Context, id uuid.UUID) error {
	now := time.Now()
	r.grants[id].RevokedAt = &now
	return nil
}
func (r *fakeRepo) ListActiveGrants(_ context.Context, userID uuid.UUID) ([]domainoauth.Grant, error) {
	var result []domainoauth.Grant
	for _, g := range r.grants {
		if g.UserID == userID && g.RevokedAt == nil {
			result = append(result, *g)
		}
	}
	return result, nil
}
func (r *fakeRepo) CreateToken(_ context.Context, t *domainoauth.Token) error {
	copy := *t
	r.tokens[t.TokenHash] = &copy
	return nil
}
func (r *fakeRepo) GetTokenByHash(_ context.Context, hash string) (*domainoauth.Token, error) {
	t, ok := r.tokens[hash]
	if !ok {
		return nil, apperror.New("OAUTH_INVALID_TOKEN")
	}
	copy := *t
	return &copy, nil
}
func (r *fakeRepo) RevokeToken(_ context.Context, id uuid.UUID, at time.Time) (bool, error) {
	for _, t := range r.tokens {
		if t.ID == id {
			if t.RevokedAt != nil {
				return false, nil
			}
			t.RevokedAt = &at
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeRepo) RevokeTokensForGrant(_ context.Context, grantID uuid.UUID) error {
	now := time.Now()
	for _, t := range r.tokens {
		if t.GrantID == grantID && t.RevokedAt == nil {
			t.RevokedAt = &now
		}
	}
	return nil
}

type fakeUsers struct{ users map[uuid.UUID]domainuser.User }

func (u fakeUsers) GetByID(_ context.Context, id uuid.UUID) (*domainuser.User, error) {
	user, ok := u.users[id]
	if !ok {
		return nil, apperror.New("USER_NOT_FOUND")
	}
	return &user, nil
}

type inlineUOW struct{}

func (inlineUOW) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fixture struct {
	svc    Service
	repo   *fakeRepo
	userID uuid.UUID
	now    *time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	repo, userID := newFakeRepo(), uuid.New()
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	svc := NewService(Dependencies{Repo: repo, UOW: inlineUOW{},
		Users:  fakeUsers{users: map[uuid.UUID]domainuser.User{userID: {ID: userID, Email: "ana@example.com", Name: "Ana"}}},
		Config: Config{Issuer: "https://tripmate.example.com/", ConsentURL: "https://tripmate.example.com/oauth/consent"},
		Clock:  func() time.Time { return now }})
	return fixture{svc: svc, repo: repo, userID: userID, now: &now}
}

const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk-long-enough-verifier"

func challenge() string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (f fixture) register(t *testing.T, method string) *RegisteredClient {
	t.Helper()
	client, err := f.svc.RegisterClient(context.Background(), RegisterInput{ClientName: "Claude",
		RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}, TokenEndpointAuthMethod: method})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// authorize runs /oauth/authorize and returns the pending request id from the consent redirect.
func (f fixture) authorize(t *testing.T, clientID, scope string) uuid.UUID {
	t.Helper()
	result, err := f.svc.Authorize(context.Background(), AuthorizeInput{ResponseType: "code", ClientID: clientID,
		RedirectURI: "https://claude.ai/api/mcp/auth_callback", Scope: scope, State: "xyz",
		CodeChallenge: challenge(), CodeChallengeMethod: "S256", Resource: "https://tripmate.example.com/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(result.RedirectURL)
	if !strings.HasPrefix(result.RedirectURL, "https://tripmate.example.com/oauth/consent?") {
		t.Fatalf("authorize redirected to %s", result.RedirectURL)
	}
	id, err := uuid.Parse(parsed.Query().Get("request_id"))
	if err != nil {
		t.Fatalf("no request id in %s", result.RedirectURL)
	}
	return id
}

func (f fixture) connect(t *testing.T, scopes []string) (*RegisteredClient, *TokenPair) {
	t.Helper()
	client := f.register(t, "none")
	requestID := f.authorize(t, client.Client.ClientID, "")
	redirect, err := f.svc.Approve(context.Background(), f.userID, requestID, scopes)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(redirect)
	if parsed.Query().Get("state") != "xyz" || parsed.Query().Get("iss") != "https://tripmate.example.com" {
		t.Fatalf("approve redirect = %s", redirect)
	}
	pair, err := f.svc.Token(context.Background(), TokenInput{GrantType: "authorization_code", Code: parsed.Query().Get("code"),
		RedirectURI: "https://claude.ai/api/mcp/auth_callback", CodeVerifier: verifier, ClientID: client.Client.ClientID})
	if err != nil {
		t.Fatal(err)
	}
	return client, pair
}

func TestFullAuthorizationCodeFlowGrantsAccessAsTheApprovingUser(t *testing.T) {
	f := newFixture(t)
	_, pair := f.connect(t, nil)
	if pair.ExpiresIn != 3600 || len(pair.Scopes) != 2 {
		t.Fatalf("pair = %+v", pair)
	}
	info, err := f.svc.VerifyAccess(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if info.User.ID != f.userID || info.ClientName != "Claude" || !domainoauth.HasScope(info.Scopes, domainoauth.ScopeWrite) {
		t.Fatalf("access = %+v", info)
	}
	if _, err := f.svc.VerifyAccess(context.Background(), pair.RefreshToken); !apperror.Is(err, "OAUTH_INVALID_TOKEN") {
		t.Fatalf("a refresh token must not work as an access token, got %v", err)
	}
}

func TestReadOnlyApprovalNarrowsTheGrant(t *testing.T) {
	f := newFixture(t)
	_, pair := f.connect(t, []string{domainoauth.ScopeRead})
	info, err := f.svc.VerifyAccess(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if domainoauth.HasScope(info.Scopes, domainoauth.ScopeWrite) || !domainoauth.HasScope(info.Scopes, domainoauth.ScopeRead) {
		t.Fatalf("scopes = %v", info.Scopes)
	}
}

func TestAuthorizationCodeIsSingleUseAndBoundToPKCEAndClient(t *testing.T) {
	f := newFixture(t)
	client := f.register(t, "none")
	other := f.register(t, "none")
	requestID := f.authorize(t, client.Client.ClientID, "tripmate.read")
	redirect, err := f.svc.Approve(context.Background(), f.userID, requestID, nil)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(redirect)
	code := parsed.Query().Get("code")
	exchange := func(clientID, codeVerifier string) error {
		_, err := f.svc.Token(context.Background(), TokenInput{GrantType: "authorization_code", Code: code, CodeVerifier: codeVerifier, ClientID: clientID})
		return err
	}
	if err := exchange(client.Client.ClientID, strings.Repeat("x", 50)); !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("wrong verifier = %v", err)
	}
	if err := exchange(other.Client.ClientID, verifier); !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("other client = %v", err)
	}
	if err := exchange(client.Client.ClientID, verifier); err != nil {
		t.Fatal(err)
	}
	if err := exchange(client.Client.ClientID, verifier); !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("code reuse = %v", err)
	}
	if _, err := f.svc.Approve(context.Background(), f.userID, requestID, nil); !apperror.Is(err, "OAUTH_REQUEST_NOT_FOUND") {
		t.Fatalf("approving twice = %v", err)
	}
}

func TestExpiredCodeAndExpiredRequestAreRefused(t *testing.T) {
	f := newFixture(t)
	client := f.register(t, "none")
	stale := f.authorize(t, client.Client.ClientID, "")
	*f.now = f.now.Add(11 * time.Minute)
	if _, err := f.svc.Approve(context.Background(), f.userID, stale, nil); !apperror.Is(err, "OAUTH_REQUEST_EXPIRED") {
		t.Fatalf("stale request = %v", err)
	}
	requestID := f.authorize(t, client.Client.ClientID, "")
	redirect, _ := f.svc.Approve(context.Background(), f.userID, requestID, nil)
	parsed, _ := url.Parse(redirect)
	*f.now = f.now.Add(2 * time.Minute)
	_, err := f.svc.Token(context.Background(), TokenInput{GrantType: "authorization_code", Code: parsed.Query().Get("code"),
		CodeVerifier: verifier, ClientID: client.Client.ClientID})
	if !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("expired code = %v", err)
	}
}

func TestRefreshRotatesAndLateReuseRevokesTheConnection(t *testing.T) {
	f := newFixture(t)
	client, first := f.connect(t, nil)
	refresh := func(token string) (*TokenPair, error) {
		return f.svc.Token(context.Background(), TokenInput{GrantType: "refresh_token", RefreshToken: token, ClientID: client.Client.ClientID})
	}
	second, err := refresh(first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	// A parallel refresh moments later is a benign race: refused, but the connection survives.
	if _, err := refresh(first.RefreshToken); !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("immediate replay = %v", err)
	}
	if _, err := f.svc.VerifyAccess(context.Background(), second.AccessToken); err != nil {
		t.Fatalf("connection should survive a benign race: %v", err)
	}
	// The same replay much later means a copy of the token leaked: everything is revoked.
	*f.now = f.now.Add(5 * time.Minute)
	if _, err := refresh(first.RefreshToken); !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("late replay = %v", err)
	}
	if _, err := f.svc.VerifyAccess(context.Background(), second.AccessToken); !apperror.Is(err, "OAUTH_INVALID_TOKEN") {
		t.Fatalf("late replay must revoke the grant, got %v", err)
	}
	if _, err := refresh(second.RefreshToken); !apperror.Is(err, "OAUTH_INVALID_GRANT") {
		t.Fatalf("refresh after revocation = %v", err)
	}
}

func TestRevokedGrantStopsAccessImmediatelyAndOnlyTheOwnerCanRevoke(t *testing.T) {
	f := newFixture(t)
	_, pair := f.connect(t, nil)
	grants, err := f.svc.ListGrants(context.Background(), f.userID)
	if err != nil || len(grants) != 1 {
		t.Fatalf("grants = %+v, %v", grants, err)
	}
	if err := f.svc.RevokeGrant(context.Background(), uuid.New(), grants[0].ID); !apperror.Is(err, "OAUTH_GRANT_NOT_FOUND") {
		t.Fatalf("revoke by someone else = %v", err)
	}
	if err := f.svc.RevokeGrant(context.Background(), f.userID, grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.VerifyAccess(context.Background(), pair.AccessToken); !apperror.Is(err, "OAUTH_INVALID_TOKEN") {
		t.Fatalf("access after revoke = %v", err)
	}
	if grants, _ := f.svc.ListGrants(context.Background(), f.userID); len(grants) != 0 {
		t.Fatalf("revoked grant still listed: %+v", grants)
	}
}

func TestReconnectingTheSameAppReusesOneGrant(t *testing.T) {
	f := newFixture(t)
	client, _ := f.connect(t, nil)
	requestID := f.authorize(t, client.Client.ClientID, "tripmate.read")
	redirect, _ := f.svc.Approve(context.Background(), f.userID, requestID, nil)
	parsed, _ := url.Parse(redirect)
	if _, err := f.svc.Token(context.Background(), TokenInput{GrantType: "authorization_code", Code: parsed.Query().Get("code"),
		CodeVerifier: verifier, ClientID: client.Client.ClientID}); err != nil {
		t.Fatal(err)
	}
	grants, _ := f.svc.ListGrants(context.Background(), f.userID)
	if len(grants) != 1 || len(grants[0].Scopes) != 1 {
		t.Fatalf("grants = %+v", grants)
	}
}

func TestIdleGrantExpires(t *testing.T) {
	f := newFixture(t)
	_, pair := f.connect(t, nil)
	*f.now = f.now.Add(91 * 24 * time.Hour)
	if _, err := f.svc.VerifyAccess(context.Background(), pair.AccessToken); !apperror.Is(err, "OAUTH_INVALID_TOKEN") {
		t.Fatalf("idle grant = %v", err)
	}
}

func TestConfidentialClientMustAuthenticate(t *testing.T) {
	f := newFixture(t)
	client := f.register(t, "")
	if client.Client.AuthMethod != domainoauth.AuthMethodBasic || client.ClientSecret == "" {
		t.Fatalf("omitted auth method should default to client_secret_basic with a secret: %+v", client)
	}
	_, err := f.svc.Token(context.Background(), TokenInput{GrantType: "refresh_token", RefreshToken: "x", ClientID: client.Client.ClientID, ClientSecret: "wrong"})
	if !apperror.Is(err, "OAUTH_INVALID_CLIENT") {
		t.Fatalf("wrong secret = %v", err)
	}
}

func TestRegistrationRejectsUnsafeRedirects(t *testing.T) {
	f := newFixture(t)
	for _, uri := range []string{"http://evil.example.com/cb", "javascript:alert(1)", "https://claude.ai/cb#frag", "data:text/html,hi"} {
		_, err := f.svc.RegisterClient(context.Background(), RegisterInput{RedirectURIs: []string{uri}, TokenEndpointAuthMethod: "none"})
		if !apperror.Is(err, "OAUTH_INVALID_REDIRECT_URI") {
			t.Errorf("%s accepted: %v", uri, err)
		}
	}
	for _, uri := range []string{"http://localhost:6274/oauth/callback", "http://127.0.0.1:33418/callback", "cursor://anysphere.cursor-mcp/oauth/callback"} {
		if _, err := f.svc.RegisterClient(context.Background(), RegisterInput{RedirectURIs: []string{uri}, TokenEndpointAuthMethod: "none"}); err != nil {
			t.Errorf("%s rejected: %v", uri, err)
		}
	}
}

func TestAuthorizeReportsErrorsSafely(t *testing.T) {
	f := newFixture(t)
	client := f.register(t, "none")
	unknown, _ := f.svc.Authorize(context.Background(), AuthorizeInput{ResponseType: "code", ClientID: "nope"})
	if !strings.Contains(unknown.RedirectURL, "/oauth/consent?error=invalid_client") {
		t.Fatalf("unknown client went to %s", unknown.RedirectURL)
	}
	foreign, _ := f.svc.Authorize(context.Background(), AuthorizeInput{ResponseType: "code", ClientID: client.Client.ClientID, RedirectURI: "https://evil.example.com/cb"})
	if !strings.Contains(foreign.RedirectURL, "/oauth/consent?error=invalid_redirect_uri") {
		t.Fatalf("unregistered redirect went to %s", foreign.RedirectURL)
	}
	noPKCE, _ := f.svc.Authorize(context.Background(), AuthorizeInput{ResponseType: "code", ClientID: client.Client.ClientID, State: "s"})
	parsed, _ := url.Parse(noPKCE.RedirectURL)
	if parsed.Host != "claude.ai" || parsed.Query().Get("error") != "invalid_request" || parsed.Query().Get("state") != "s" {
		t.Fatalf("missing PKCE went to %s", noPKCE.RedirectURL)
	}
	badScope, _ := f.svc.Authorize(context.Background(), AuthorizeInput{ResponseType: "code", ClientID: client.Client.ClientID,
		CodeChallenge: challenge(), CodeChallengeMethod: "S256", Scope: "admin"})
	if parsed, _ := url.Parse(badScope.RedirectURL); parsed.Query().Get("error") != "invalid_scope" {
		t.Fatalf("bad scope went to %s", badScope.RedirectURL)
	}
}

func TestDenyReturnsAccessDenied(t *testing.T) {
	f := newFixture(t)
	client := f.register(t, "none")
	requestID := f.authorize(t, client.Client.ClientID, "")
	redirect, err := f.svc.Deny(context.Background(), f.userID, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, _ := url.Parse(redirect); parsed.Query().Get("error") != "access_denied" || parsed.Query().Get("state") != "xyz" {
		t.Fatalf("deny redirect = %s", redirect)
	}
}
