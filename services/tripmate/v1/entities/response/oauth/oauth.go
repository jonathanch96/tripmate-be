package oauthresponse

import (
	"net/url"
	"time"

	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
)

type Client struct {
	Name      string  `json:"name"`
	ClientURI *string `json:"client_uri"`
	LogoURI   *string `json:"logo_uri"`
}

// AuthorizationRequest is what the consent page shows the signed-in user.
type AuthorizationRequest struct {
	ID           string    `json:"id"`
	Client       Client    `json:"client"`
	RedirectHost string    `json:"redirect_host"`
	Scopes       []string  `json:"scopes"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type Redirect struct {
	RedirectURL string `json:"redirect_url"`
}

// Grant is one connected app on the account page.
type Grant struct {
	ID         string    `json:"id"`
	Client     Client    `json:"client"`
	Scopes     []string  `json:"scopes"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

func FromRequest(request domainoauth.AuthRequest, client domainoauth.Client) AuthorizationRequest {
	host := request.RedirectURI
	if parsed, err := url.Parse(request.RedirectURI); err == nil {
		if parsed.Host != "" {
			host = parsed.Host
		} else if parsed.Scheme != "" {
			host = parsed.Scheme + ":"
		}
	}
	return AuthorizationRequest{ID: request.ID.String(), Client: fromClient(client), RedirectHost: host,
		Scopes: append([]string{}, request.Scopes...), ExpiresAt: request.ExpiresAt}
}

func FromGrants(rows []domainoauth.Grant) []Grant {
	result := make([]Grant, 0, len(rows))
	for _, grant := range rows {
		item := Grant{ID: grant.ID.String(), Scopes: append([]string{}, grant.Scopes...), CreatedAt: grant.CreatedAt, LastUsedAt: grant.LastUsedAt}
		if grant.Client != nil {
			item.Client = fromClient(*grant.Client)
		}
		result = append(result, item)
	}
	return result
}

func fromClient(client domainoauth.Client) Client {
	return Client{Name: client.Name, ClientURI: client.ClientURI, LogoURI: client.LogoURI}
}
