package oauthrequest

// Register is a dynamic client registration request (RFC 7591 §2).
type Register struct {
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	LogoURI                 string   `json:"logo_uri"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

// Approve is the consent page's answer. Scopes may narrow the request (e.g. read-only); empty
// approves everything that was asked for.
type Approve struct {
	Scopes []string `json:"scopes"`
}
