package oauth

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	oauthdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/oauth"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
	oauthrequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/oauth"
	oauthresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/oauth"
)

func NewController(service oauthdomain.Service) Controller {
	// AI tools register and refresh from a handful of shared provider IPs, so these limits are per
	// IP for registration and per client for the token endpoint, and deliberately generous.
	return &controller{service: service,
		registerIP: middleware.NewRateLimiter(300, time.Hour), tokenClient: middleware.NewRateLimiter(600, time.Hour)}
}

// RegisterProtocolRoutes mounts the OAuth endpoints AI tools call directly, at the root of the
// public origin rather than under /api/v1, and with the RFC 6749 error format instead of the
// envelope.
func (c *controller) RegisterProtocolRoutes(engine *gin.Engine) {
	engine.GET("/.well-known/oauth-protected-resource", c.resourceMetadata)
	engine.GET("/.well-known/oauth-protected-resource/mcp", c.resourceMetadata)
	engine.GET("/.well-known/oauth-authorization-server", c.serverMetadata)
	engine.POST("/oauth/register", middleware.RateLimit(c.registerIP, func(ctx *gin.Context) string { return ctx.ClientIP() }), c.register)
	engine.GET("/oauth/authorize", c.authorize)
	engine.POST("/oauth/token", middleware.RateLimit(c.tokenClient, tokenKey), c.token)
}

// RegisterRoutes mounts the endpoints the TripMate frontend calls for the signed-in user.
func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	routes := group.Group("/oauth")
	routes.GET("/requests/:id", c.getRequest)
	routes.POST("/requests/:id/approve", c.approve)
	routes.POST("/requests/:id/deny", c.deny)
	routes.GET("/grants", c.listGrants)
	routes.DELETE("/grants/:id", c.revokeGrant)
}

func (c *controller) resourceMetadata(ctx *gin.Context) {
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.JSON(http.StatusOK, gin.H{
		"resource":                 c.service.Resource(),
		"authorization_servers":    []string{c.service.Issuer()},
		"scopes_supported":         domainoauth.SupportedScopes,
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "TripMate",
	})
}

func (c *controller) serverMetadata(ctx *gin.Context) {
	issuer := c.service.Issuer()
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.JSON(http.StatusOK, gin.H{
		"issuer":                                         issuer,
		"authorization_endpoint":                         issuer + "/oauth/authorize",
		"token_endpoint":                                 issuer + "/oauth/token",
		"registration_endpoint":                          issuer + "/oauth/register",
		"scopes_supported":                               domainoauth.SupportedScopes,
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported":          []string{"none", "client_secret_post", "client_secret_basic"},
		"code_challenge_methods_supported":               []string{"S256"},
		"authorization_response_iss_parameter_supported": true,
	})
}

func (c *controller) register(ctx *gin.Context) {
	var req oauthrequest.Register
	if err := ctx.ShouldBindJSON(&req); err != nil {
		oauthError(ctx, apperror.Newf("OAUTH_INVALID_CLIENT_METADATA", "request body must be JSON client metadata"))
		return
	}
	registered, err := c.service.RegisterClient(ctx, oauthdomain.RegisterInput{
		ClientName: req.ClientName, ClientURI: req.ClientURI, LogoURI: req.LogoURI, RedirectURIs: req.RedirectURIs,
		TokenEndpointAuthMethod: req.TokenEndpointAuthMethod, GrantTypes: req.GrantTypes, ResponseTypes: req.ResponseTypes,
	})
	if err != nil {
		oauthError(ctx, err)
		return
	}
	client := registered.Client
	body := gin.H{
		"client_id":                  client.ClientID,
		"client_id_issued_at":        client.CreatedAt.Unix(),
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": string(client.AuthMethod),
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	}
	if client.ClientURI != nil {
		body["client_uri"] = *client.ClientURI
	}
	if client.LogoURI != nil {
		body["logo_uri"] = *client.LogoURI
	}
	if registered.ClientSecret != "" {
		body["client_secret"] = registered.ClientSecret
		body["client_secret_expires_at"] = 0
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusCreated, body)
}

func (c *controller) authorize(ctx *gin.Context) {
	result, err := c.service.Authorize(ctx, oauthdomain.AuthorizeInput{
		ResponseType: ctx.Query("response_type"), ClientID: ctx.Query("client_id"), RedirectURI: ctx.Query("redirect_uri"),
		Scope: ctx.Query("scope"), State: ctx.Query("state"), CodeChallenge: ctx.Query("code_challenge"),
		CodeChallengeMethod: ctx.Query("code_challenge_method"), Resource: ctx.Query("resource"),
	})
	if err != nil {
		oauthError(ctx, err)
		return
	}
	ctx.Redirect(http.StatusFound, result.RedirectURL)
}

func (c *controller) token(ctx *gin.Context) {
	clientID, clientSecret, basic := ctx.Request.BasicAuth()
	if !basic {
		clientID, clientSecret = ctx.PostForm("client_id"), ctx.PostForm("client_secret")
	}
	pair, err := c.service.Token(ctx, oauthdomain.TokenInput{
		GrantType: ctx.PostForm("grant_type"), Code: ctx.PostForm("code"), RedirectURI: ctx.PostForm("redirect_uri"),
		CodeVerifier: ctx.PostForm("code_verifier"), RefreshToken: ctx.PostForm("refresh_token"),
		ClientID: clientID, ClientSecret: clientSecret, Resource: ctx.PostForm("resource"), Scope: ctx.PostForm("scope"),
	})
	if err != nil {
		oauthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusOK, gin.H{
		"access_token": pair.AccessToken, "token_type": "Bearer", "expires_in": pair.ExpiresIn,
		"refresh_token": pair.RefreshToken, "scope": joinScopes(pair.Scopes),
	})
}

// getRequest godoc
// @Summary Get a pending AI tool authorization request
// @Description Used by the consent page to show which app is asking for access and what it can do.
// @Tags oauth
// @Security BearerAuth
// @Param id path string true "Authorization request ID"
// @Success 200 {object} response.Envelope{data=oauthresponse.AuthorizationRequest}
// @Failure 404 {object} response.Envelope
// @Failure 410 {object} response.Envelope
// @Router /oauth/requests/{id} [get]
func (c *controller) getRequest(ctx *gin.Context) {
	id, ok := requestID(ctx)
	if !ok {
		return
	}
	view, err := c.service.GetRequest(ctx, identity.MustFromContext(ctx.Request.Context()).UserID, id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "OAUTH_REQUEST_FETCHED", oauthresponse.FromRequest(view.Request, view.Client))
}

// approve godoc
// @Summary Approve an AI tool authorization request
// @Description Grants the app access to the signed-in account and returns where to send the browser.
// @Tags oauth
// @Security BearerAuth
// @Param id path string true "Authorization request ID"
// @Param body body oauthrequest.Approve true "Scopes to grant (empty grants everything requested)"
// @Success 200 {object} response.Envelope{data=oauthresponse.Redirect}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 410 {object} response.Envelope
// @Router /oauth/requests/{id}/approve [post]
func (c *controller) approve(ctx *gin.Context) {
	id, ok := requestID(ctx)
	if !ok {
		return
	}
	var req oauthrequest.Approve
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			response.Error(ctx, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "request", Rule: "invalid", Message: err.Error()}}))
			return
		}
	}
	redirect, err := c.service.Approve(ctx, identity.MustFromContext(ctx.Request.Context()).UserID, id, req.Scopes)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "OAUTH_REQUEST_APPROVED", oauthresponse.Redirect{RedirectURL: redirect})
}

// deny godoc
// @Summary Deny an AI tool authorization request
// @Tags oauth
// @Security BearerAuth
// @Param id path string true "Authorization request ID"
// @Success 200 {object} response.Envelope{data=oauthresponse.Redirect}
// @Failure 404 {object} response.Envelope
// @Failure 410 {object} response.Envelope
// @Router /oauth/requests/{id}/deny [post]
func (c *controller) deny(ctx *gin.Context) {
	id, ok := requestID(ctx)
	if !ok {
		return
	}
	redirect, err := c.service.Deny(ctx, identity.MustFromContext(ctx.Request.Context()).UserID, id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "OAUTH_REQUEST_DENIED", oauthresponse.Redirect{RedirectURL: redirect})
}

// listGrants godoc
// @Summary List connected AI apps
// @Tags oauth
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=[]oauthresponse.Grant}
// @Router /oauth/grants [get]
func (c *controller) listGrants(ctx *gin.Context) {
	rows, err := c.service.ListGrants(ctx, identity.MustFromContext(ctx.Request.Context()).UserID)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "OAUTH_GRANTS_FETCHED", oauthresponse.FromGrants(rows))
}

// revokeGrant godoc
// @Summary Disconnect an AI app
// @Description Revokes the connection and every token it holds, effective immediately.
// @Tags oauth
// @Security BearerAuth
// @Param id path string true "Grant ID"
// @Success 200 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /oauth/grants/{id} [delete]
func (c *controller) revokeGrant(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("OAUTH_GRANT_NOT_FOUND"))
		return
	}
	if err := c.service.RevokeGrant(ctx, identity.MustFromContext(ctx.Request.Context()).UserID, id); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "OAUTH_GRANT_REVOKED")
}

func requestID(ctx *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("OAUTH_REQUEST_NOT_FOUND"))
		return uuid.Nil, false
	}
	return id, true
}

func tokenKey(ctx *gin.Context) string {
	if clientID, _, ok := ctx.Request.BasicAuth(); ok && clientID != "" {
		return "client:" + clientID
	}
	if clientID := ctx.PostForm("client_id"); clientID != "" {
		return "client:" + clientID
	}
	return "ip:" + ctx.ClientIP()
}

// oauthErrorCodes maps the domain's error codes onto RFC 6749 §5.2 / RFC 7591 §3.2.2 error values.
var oauthErrorCodes = map[string]string{
	"OAUTH_INVALID_REQUEST":         "invalid_request",
	"OAUTH_INVALID_CLIENT":          "invalid_client",
	"OAUTH_INVALID_GRANT":           "invalid_grant",
	"OAUTH_INVALID_SCOPE":           "invalid_scope",
	"OAUTH_INVALID_TARGET":          "invalid_target",
	"OAUTH_UNSUPPORTED_GRANT_TYPE":  "unsupported_grant_type",
	"OAUTH_INVALID_REDIRECT_URI":    "invalid_redirect_uri",
	"OAUTH_INVALID_CLIENT_METADATA": "invalid_client_metadata",
	"RATE_LIMITED":                  "temporarily_unavailable",
}

func oauthError(ctx *gin.Context, err error) {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		appErr = apperror.Wrap(err, "INTERNAL_ERROR")
	}
	code, known := oauthErrorCodes[appErr.Code]
	status := appErr.HTTP
	if !known {
		code, status = "server_error", http.StatusInternalServerError
		_ = ctx.Error(err)
	}
	if code == "invalid_client" {
		ctx.Header("WWW-Authenticate", `Basic realm="tripmate"`)
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(status, gin.H{"error": code, "error_description": appErr.Message})
}

func joinScopes(scopes []string) string {
	result := ""
	for index, scope := range scopes {
		if index > 0 {
			result += " "
		}
		result += scope
	}
	return result
}
