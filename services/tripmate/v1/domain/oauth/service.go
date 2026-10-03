package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
)

const (
	defaultAccessTTL    = time.Hour
	defaultRefreshTTL   = 30 * 24 * time.Hour
	defaultCodeTTL      = time.Minute
	defaultRequestTTL   = 10 * time.Minute
	defaultGrantIdleTTL = 90 * 24 * time.Hour
	// refreshReuseGrace tolerates an AI tool refreshing twice in parallel: replaying a refresh token
	// this soon after its rotation is treated as a benign race, not theft, so the grant survives.
	refreshReuseGrace = time.Minute
	// touchInterval throttles last_used_at writes to one per grant every few minutes.
	touchInterval    = 5 * time.Minute
	maxRedirectURIs  = 10
	maxClientNameLen = 120
)

type service struct {
	deps Dependencies
	cfg  Config
}

func NewService(deps Dependencies) Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	cfg := deps.Config
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = defaultAccessTTL
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = defaultRefreshTTL
	}
	if cfg.CodeTTL <= 0 {
		cfg.CodeTTL = defaultCodeTTL
	}
	if cfg.RequestTTL <= 0 {
		cfg.RequestTTL = defaultRequestTTL
	}
	if cfg.GrantIdleTTL <= 0 {
		cfg.GrantIdleTTL = defaultGrantIdleTTL
	}
	return &service{deps: deps, cfg: cfg}
}

func (s *service) Issuer() string   { return s.cfg.Issuer }
func (s *service) Resource() string { return s.cfg.Issuer + "/mcp" }

func (s *service) now() time.Time { return s.deps.Clock().UTC() }

// RegisterClient implements dynamic client registration (RFC 7591). Any AI tool may register; what
// protects users is the consent page, which names the tool and where it will send them.
func (s *service) RegisterClient(ctx context.Context, in RegisterInput) (*RegisteredClient, error) {
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > maxRedirectURIs {
		return nil, apperror.Newf("OAUTH_INVALID_REDIRECT_URI", "between 1 and %d redirect_uris are required", maxRedirectURIs)
	}
	for _, raw := range in.RedirectURIs {
		if !validRedirectURI(raw) {
			return nil, apperror.Newf("OAUTH_INVALID_REDIRECT_URI", "redirect_uri %q is not allowed", raw)
		}
	}
	for _, grantType := range in.GrantTypes {
		if grantType != "authorization_code" && grantType != "refresh_token" {
			return nil, apperror.Newf("OAUTH_INVALID_CLIENT_METADATA", "grant_type %q is not supported", grantType)
		}
	}
	for _, responseType := range in.ResponseTypes {
		if responseType != "code" {
			return nil, apperror.Newf("OAUTH_INVALID_CLIENT_METADATA", "response_type %q is not supported", responseType)
		}
	}
	// RFC 7591 §2: an omitted token_endpoint_auth_method means client_secret_basic.
	method := domainoauth.AuthMethod(in.TokenEndpointAuthMethod)
	if method == "" {
		method = domainoauth.AuthMethodBasic
	}
	if method != domainoauth.AuthMethodNone && method != domainoauth.AuthMethodPost && method != domainoauth.AuthMethodBasic {
		return nil, apperror.Newf("OAUTH_INVALID_CLIENT_METADATA", "token_endpoint_auth_method %q is not supported", method)
	}
	clientID, err := randomToken("tmc_", 16)
	if err != nil {
		return nil, err
	}
	entity := &domainoauth.Client{
		ID: uuid.New(), ClientID: clientID, Name: clientName(in), RedirectURIs: slices.Clone(in.RedirectURIs),
		AuthMethod: method, ClientURI: httpsURL(in.ClientURI), LogoURI: httpsURL(in.LogoURI), CreatedAt: s.now(),
	}
	var secret string
	if method != domainoauth.AuthMethodNone {
		if secret, err = randomToken("tms_", 32); err != nil {
			return nil, err
		}
		hash := hashToken(secret)
		entity.ClientSecretHash = &hash
	}
	created, err := s.deps.Repo.CreateClient(ctx, entity)
	if err != nil {
		return nil, err
	}
	return &RegisteredClient{Client: *created, ClientSecret: secret}, nil
}

// Authorize validates an /oauth/authorize request and parks it for the consent page. Problems with
// the client or redirect_uri cannot be reported to the client safely, so they go to the consent
// page as an error; everything else is reported back on the redirect_uri (RFC 6749 §4.1.2.1).
func (s *service) Authorize(ctx context.Context, in AuthorizeInput) (*AuthorizeResult, error) {
	client, err := s.deps.Repo.GetClientByClientID(ctx, in.ClientID)
	if err != nil {
		if apperror.Is(err, "OAUTH_INVALID_CLIENT") {
			return &AuthorizeResult{RedirectURL: s.consentError("invalid_client")}, nil
		}
		return nil, err
	}
	redirectURI := in.RedirectURI
	if redirectURI == "" && len(client.RedirectURIs) == 1 {
		redirectURI = client.RedirectURIs[0]
	}
	if !slices.Contains(client.RedirectURIs, redirectURI) {
		return &AuthorizeResult{RedirectURL: s.consentError("invalid_redirect_uri")}, nil
	}
	fail := func(code, description string) (*AuthorizeResult, error) {
		return &AuthorizeResult{RedirectURL: s.clientRedirect(redirectURI, url.Values{
			"error": {code}, "error_description": {description}}, in.State)}, nil
	}
	if in.ResponseType != "code" {
		return fail("unsupported_response_type", "only response_type=code is supported")
	}
	if in.CodeChallenge == "" || in.CodeChallengeMethod != "S256" {
		return fail("invalid_request", "PKCE with code_challenge_method=S256 is required")
	}
	scopes, ok := parseScopes(in.Scope)
	if !ok {
		return fail("invalid_scope", "unknown scope requested")
	}
	if in.Resource != "" && !s.matchesResource(in.Resource) {
		return fail("invalid_target", "resource must be "+s.Resource())
	}
	request := &domainoauth.AuthRequest{
		ID: uuid.New(), ClientID: client.ID, RedirectURI: redirectURI, Scopes: scopes,
		CodeChallenge: in.CodeChallenge, Status: domainoauth.RequestPending,
		ExpiresAt: s.now().Add(s.cfg.RequestTTL), CreatedAt: s.now(),
	}
	if in.State != "" {
		request.State = &in.State
	}
	if in.Resource != "" {
		request.Resource = &in.Resource
	}
	if _, err := s.deps.Repo.CreateRequest(ctx, request); err != nil {
		return nil, err
	}
	return &AuthorizeResult{RedirectURL: s.cfg.ConsentURL + "?" + url.Values{"request_id": {request.ID.String()}}.Encode()}, nil
}

func (s *service) GetRequest(ctx context.Context, _ uuid.UUID, requestID uuid.UUID) (*RequestView, error) {
	request, err := s.pendingRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	return &RequestView{Request: *request, Client: *request.Client}, nil
}

// Approve records the signed-in user's consent and returns the client redirect carrying the
// authorization code. scopes is what the user chose on the consent page and may differ from the
// request either way (RFC 6749 §3.3): read-only for an app that asked to edit, or "Create and edit"
// for an app that only asked to view. The token response reports the scopes actually granted.
// Empty grants what was asked.
func (s *service) Approve(ctx context.Context, userID, requestID uuid.UUID, scopes []string) (string, error) {
	request, err := s.pendingRequest(ctx, requestID)
	if err != nil {
		return "", err
	}
	granted := request.Scopes
	if len(scopes) > 0 {
		chosen, ok := parseScopes(strings.Join(scopes, " "))
		if !ok {
			return "", apperror.New("OAUTH_INVALID_SCOPE")
		}
		granted = chosen
	}
	code, err := randomToken("tmac_", 32)
	if err != nil {
		return "", err
	}
	hash, expires := hashToken(code), s.now().Add(s.cfg.CodeTTL)
	request.Status, request.UserID, request.GrantedScopes = domainoauth.RequestApproved, &userID, granted
	request.CodeHash, request.CodeExpiresAt = &hash, &expires
	won, err := s.deps.Repo.UpdateRequestStatus(ctx, request, domainoauth.RequestPending)
	if err != nil {
		return "", err
	}
	if !won {
		return "", apperror.New("OAUTH_REQUEST_NOT_FOUND")
	}
	return s.clientRedirect(request.RedirectURI, url.Values{"code": {code}}, deref(request.State)), nil
}

func (s *service) Deny(ctx context.Context, userID, requestID uuid.UUID) (string, error) {
	request, err := s.pendingRequest(ctx, requestID)
	if err != nil {
		return "", err
	}
	request.Status, request.UserID = domainoauth.RequestDenied, &userID
	won, err := s.deps.Repo.UpdateRequestStatus(ctx, request, domainoauth.RequestPending)
	if err != nil {
		return "", err
	}
	if !won {
		return "", apperror.New("OAUTH_REQUEST_NOT_FOUND")
	}
	return s.clientRedirect(request.RedirectURI, url.Values{
		"error": {"access_denied"}, "error_description": {"the user declined the request"}}, deref(request.State)), nil
}

func (s *service) Token(ctx context.Context, in TokenInput) (*TokenPair, error) {
	client, err := s.authenticateClient(ctx, in.ClientID, in.ClientSecret)
	if err != nil {
		return nil, err
	}
	if in.Resource != "" && !s.matchesResource(in.Resource) {
		return nil, apperror.Newf("OAUTH_INVALID_TARGET", "resource must be %s", s.Resource())
	}
	switch in.GrantType {
	case "authorization_code":
		return s.exchangeCode(ctx, client, in)
	case "refresh_token":
		return s.refresh(ctx, client, in)
	default:
		return nil, apperror.New("OAUTH_UNSUPPORTED_GRANT_TYPE")
	}
}

func (s *service) exchangeCode(ctx context.Context, client *domainoauth.Client, in TokenInput) (*TokenPair, error) {
	if in.Code == "" || in.CodeVerifier == "" {
		return nil, apperror.Newf("OAUTH_INVALID_REQUEST", "code and code_verifier are required")
	}
	request, err := s.deps.Repo.GetRequestByCodeHash(ctx, hashToken(in.Code))
	if err != nil {
		return nil, err
	}
	switch {
	case request.Status != domainoauth.RequestApproved || request.UserID == nil || request.CodeExpiresAt == nil:
		return nil, apperror.Newf("OAUTH_INVALID_GRANT", "authorization code is invalid or already used")
	case request.ClientID != client.ID:
		return nil, apperror.Newf("OAUTH_INVALID_GRANT", "authorization code was issued to another client")
	case !request.CodeExpiresAt.After(s.now()):
		return nil, apperror.Newf("OAUTH_INVALID_GRANT", "authorization code has expired")
	case in.RedirectURI != "" && in.RedirectURI != request.RedirectURI:
		return nil, apperror.Newf("OAUTH_INVALID_GRANT", "redirect_uri does not match the authorization request")
	case !verifyPKCE(in.CodeVerifier, request.CodeChallenge):
		return nil, apperror.Newf("OAUTH_INVALID_GRANT", "code_verifier does not match code_challenge")
	}
	var pair *TokenPair
	err = s.deps.UOW.Do(ctx, func(ctx context.Context) error {
		won, err := s.deps.Repo.UpdateRequestStatus(ctx, &domainoauth.AuthRequest{
			ID: request.ID, Status: domainoauth.RequestUsed, UserID: request.UserID, GrantedScopes: request.GrantedScopes,
			CodeHash: request.CodeHash, CodeExpiresAt: request.CodeExpiresAt,
		}, domainoauth.RequestApproved)
		if err != nil {
			return err
		}
		if !won {
			return apperror.Newf("OAUTH_INVALID_GRANT", "authorization code is invalid or already used")
		}
		grant, err := s.upsertGrant(ctx, *request.UserID, client.ID, request.GrantedScopes)
		if err != nil {
			return err
		}
		pair, err = s.issue(ctx, grant)
		return err
	})
	return pair, err
}

func (s *service) refresh(ctx context.Context, client *domainoauth.Client, in TokenInput) (*TokenPair, error) {
	invalid := apperror.Newf("OAUTH_INVALID_GRANT", "refresh token is invalid or expired")
	token, err := s.deps.Repo.GetTokenByHash(ctx, hashToken(in.RefreshToken))
	if err != nil || token.Kind != domainoauth.TokenRefresh {
		return nil, invalid
	}
	grant, err := s.deps.Repo.GetGrant(ctx, token.GrantID)
	if err != nil {
		return nil, invalid
	}
	if grant.ClientID != client.ID || !s.grantActive(grant) {
		return nil, invalid
	}
	if token.RevokedAt != nil {
		if s.now().Sub(*token.RevokedAt) > refreshReuseGrace {
			// A rotated refresh token came back long after it was replaced: someone else holds a
			// copy. Cut the whole connection off (OAuth 2.1 §4.3.1).
			if err := s.revokeGrant(ctx, grant.ID); err != nil {
				return nil, err
			}
		}
		return nil, invalid
	}
	if !token.ExpiresAt.After(s.now()) {
		return nil, invalid
	}
	if in.Scope != "" {
		requested, ok := parseScopes(in.Scope)
		if !ok {
			return nil, apperror.New("OAUTH_INVALID_SCOPE")
		}
		for _, scope := range requested {
			if !slices.Contains(grant.Scopes, scope) {
				return nil, apperror.New("OAUTH_INVALID_SCOPE")
			}
		}
	}
	var pair *TokenPair
	err = s.deps.UOW.Do(ctx, func(ctx context.Context) error {
		won, err := s.deps.Repo.RevokeToken(ctx, token.ID, s.now())
		if err != nil {
			return err
		}
		if !won {
			return invalid
		}
		pair, err = s.issue(ctx, grant)
		return err
	})
	return pair, err
}

func (s *service) VerifyAccess(ctx context.Context, raw string) (*AccessInfo, error) {
	invalid := apperror.New("OAUTH_INVALID_TOKEN")
	if raw == "" {
		return nil, invalid
	}
	token, err := s.deps.Repo.GetTokenByHash(ctx, hashToken(raw))
	if err != nil {
		if apperror.Is(err, "OAUTH_INVALID_TOKEN") {
			return nil, invalid
		}
		return nil, err
	}
	if token.Kind != domainoauth.TokenAccess || token.RevokedAt != nil || !token.ExpiresAt.After(s.now()) {
		return nil, invalid
	}
	grant, err := s.deps.Repo.GetGrant(ctx, token.GrantID)
	if err != nil {
		return nil, invalid
	}
	if !s.grantActive(grant) {
		return nil, invalid
	}
	user, err := s.deps.Users.GetByID(ctx, grant.UserID)
	if err != nil {
		return nil, invalid
	}
	if s.now().Sub(grant.LastUsedAt) > touchInterval {
		// Best effort: a failed touch must not fail the request it rides on.
		_ = s.deps.Repo.TouchGrant(ctx, grant.ID, s.now())
	}
	name := ""
	if grant.Client != nil {
		name = grant.Client.Name
	}
	return &AccessInfo{User: *user, GrantID: grant.ID, ClientName: name, Scopes: slices.Clone(grant.Scopes), ExpiresAt: token.ExpiresAt}, nil
}

func (s *service) ListGrants(ctx context.Context, userID uuid.UUID) ([]domainoauth.Grant, error) {
	rows, err := s.deps.Repo.ListActiveGrants(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := make([]domainoauth.Grant, 0, len(rows))
	for _, grant := range rows {
		if s.grantActive(&grant) {
			active = append(active, grant)
		}
	}
	return active, nil
}

func (s *service) RevokeGrant(ctx context.Context, userID, grantID uuid.UUID) error {
	grant, err := s.deps.Repo.GetGrant(ctx, grantID)
	if err != nil {
		return err
	}
	if grant.UserID != userID || grant.RevokedAt != nil {
		return apperror.New("OAUTH_GRANT_NOT_FOUND")
	}
	return s.revokeGrant(ctx, grantID)
}

func (s *service) revokeGrant(ctx context.Context, grantID uuid.UUID) error {
	return s.deps.UOW.Do(ctx, func(ctx context.Context) error {
		if err := s.deps.Repo.RevokeGrant(ctx, grantID); err != nil {
			return err
		}
		return s.deps.Repo.RevokeTokensForGrant(ctx, grantID)
	})
}

func (s *service) pendingRequest(ctx context.Context, requestID uuid.UUID) (*domainoauth.AuthRequest, error) {
	request, err := s.deps.Repo.GetRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if request.Status != domainoauth.RequestPending || request.Client == nil {
		return nil, apperror.New("OAUTH_REQUEST_NOT_FOUND")
	}
	if !request.ExpiresAt.After(s.now()) {
		return nil, apperror.New("OAUTH_REQUEST_EXPIRED")
	}
	return request, nil
}

func (s *service) authenticateClient(ctx context.Context, clientID, secret string) (*domainoauth.Client, error) {
	if clientID == "" {
		return nil, apperror.New("OAUTH_INVALID_CLIENT")
	}
	client, err := s.deps.Repo.GetClientByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if client.AuthMethod == domainoauth.AuthMethodNone {
		return client, nil
	}
	if client.ClientSecretHash == nil || subtle.ConstantTimeCompare([]byte(hashToken(secret)), []byte(*client.ClientSecretHash)) != 1 {
		return nil, apperror.New("OAUTH_INVALID_CLIENT")
	}
	return client, nil
}

// upsertGrant keeps one active connection per user and client: approving the same tool again
// updates its scopes instead of stacking a second grant.
func (s *service) upsertGrant(ctx context.Context, userID, clientID uuid.UUID, scopes []string) (*domainoauth.Grant, error) {
	existing, err := s.deps.Repo.GetActiveGrant(ctx, userID, clientID)
	switch {
	case err == nil && s.grantActive(existing):
		if err := s.deps.Repo.UpdateGrantScopes(ctx, existing.ID, scopes); err != nil {
			return nil, err
		}
		if err := s.deps.Repo.TouchGrant(ctx, existing.ID, s.now()); err != nil {
			return nil, err
		}
		existing.Scopes = scopes
		return existing, nil
	case err == nil:
		// Idle past its lifetime: retire it so the unique index admits a fresh one.
		if err := s.revokeGrant(ctx, existing.ID); err != nil {
			return nil, err
		}
	case !apperror.Is(err, "OAUTH_GRANT_NOT_FOUND"):
		return nil, err
	}
	now := s.now()
	return s.deps.Repo.CreateGrant(ctx, &domainoauth.Grant{ID: uuid.New(), ClientID: clientID, UserID: userID,
		Scopes: scopes, CreatedAt: now, LastUsedAt: now})
}

func (s *service) issue(ctx context.Context, grant *domainoauth.Grant) (*TokenPair, error) {
	access, err := randomToken("tmat_", 32)
	if err != nil {
		return nil, err
	}
	refresh, err := randomToken("tmrt_", 32)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for _, token := range []domainoauth.Token{
		{ID: uuid.New(), GrantID: grant.ID, Kind: domainoauth.TokenAccess, TokenHash: hashToken(access), ExpiresAt: now.Add(s.cfg.AccessTTL), CreatedAt: now},
		{ID: uuid.New(), GrantID: grant.ID, Kind: domainoauth.TokenRefresh, TokenHash: hashToken(refresh), ExpiresAt: now.Add(s.cfg.RefreshTTL), CreatedAt: now},
	} {
		if err := s.deps.Repo.CreateToken(ctx, &token); err != nil {
			return nil, err
		}
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int(s.cfg.AccessTTL.Seconds()), Scopes: slices.Clone(grant.Scopes)}, nil
}

func (s *service) grantActive(grant *domainoauth.Grant) bool {
	return grant.RevokedAt == nil && s.now().Sub(grant.LastUsedAt) <= s.cfg.GrantIdleTTL
}

func (s *service) matchesResource(resource string) bool {
	return strings.TrimRight(resource, "/") == s.Resource() || strings.TrimRight(resource, "/") == s.cfg.Issuer
}

func (s *service) consentError(code string) string {
	return s.cfg.ConsentURL + "?" + url.Values{"error": {code}}.Encode()
}

// clientRedirect appends params (plus state and iss, RFC 9207) to a registered redirect_uri,
// keeping any query it already has.
func (s *service) clientRedirect(redirectURI string, params url.Values, state string) string {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	query := target.Query()
	for key, values := range params {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	if state != "" {
		query.Set("state", state)
	}
	query.Set("iss", s.cfg.Issuer)
	target.RawQuery = query.Encode()
	return target.String()
}

// parseScopes turns a space-separated scope string into the stored set. Empty asks for
// everything; write always brings read along.
func parseScopes(raw string) ([]string, bool) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return slices.Clone(domainoauth.SupportedScopes), true
	}
	set := map[string]bool{}
	for _, field := range fields {
		if !slices.Contains(domainoauth.SupportedScopes, field) {
			return nil, false
		}
		set[field] = true
	}
	if set[domainoauth.ScopeWrite] {
		set[domainoauth.ScopeRead] = true
	}
	scopes := make([]string, 0, len(set))
	for _, scope := range domainoauth.SupportedScopes {
		if set[scope] {
			scopes = append(scopes, scope)
		}
	}
	return scopes, true
}

// validRedirectURI accepts https, http on loopback (local tools such as MCP Inspector), and
// private-use schemes for native apps (RFC 8252 §7.1). Fragments and script-capable schemes are
// refused.
func validRedirectURI(raw string) bool {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Fragment != "" {
		return false
	}
	switch strings.ToLower(target.Scheme) {
	case "https":
		return target.Host != ""
	case "http":
		host := target.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	case "javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss":
		return false
	default:
		return true
	}
}

func clientName(in RegisterInput) string {
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		if parsed, err := url.Parse(in.ClientURI); err == nil && parsed.Host != "" {
			name = parsed.Host
		} else {
			name = "Unnamed app"
		}
	}
	if runes := []rune(name); len(runes) > maxClientNameLen {
		name = string(runes[:maxClientNameLen])
	}
	return name
}

func httpsURL(raw string) *string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil
	}
	value := parsed.String()
	return &value
}

func verifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	digest := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

func randomToken(prefix string, size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashToken(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
