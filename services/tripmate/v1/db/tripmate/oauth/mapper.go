package oauth

import (
	"slices"

	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
)

func clientToDomain(model Client) domainoauth.Client {
	return domainoauth.Client{
		ID: model.ID, ClientID: model.ClientID, ClientSecretHash: model.ClientSecretHash, Name: model.Name,
		ClientURI: model.ClientURI, LogoURI: model.LogoURI, RedirectURIs: slices.Clone([]string(model.RedirectURIs)),
		AuthMethod: domainoauth.AuthMethod(model.TokenEndpointAuthMethod), CreatedAt: model.CreatedAt,
	}
}

func clientFromDomain(entity domainoauth.Client) Client {
	return Client{
		ID: entity.ID, ClientID: entity.ClientID, ClientSecretHash: entity.ClientSecretHash, Name: entity.Name,
		ClientURI: entity.ClientURI, LogoURI: entity.LogoURI, RedirectURIs: slices.Clone(entity.RedirectURIs),
		TokenEndpointAuthMethod: string(entity.AuthMethod), CreatedAt: entity.CreatedAt,
	}
}

func requestToDomain(model AuthRequest, client *Client) domainoauth.AuthRequest {
	entity := domainoauth.AuthRequest{
		ID: model.ID, ClientID: model.ClientID, RedirectURI: model.RedirectURI, Scopes: slices.Clone([]string(model.Scopes)),
		State: model.State, CodeChallenge: model.CodeChallenge, Resource: model.Resource,
		Status: domainoauth.RequestStatus(model.Status), UserID: model.UserID,
		GrantedScopes: slices.Clone([]string(model.GrantedScopes)), CodeHash: model.CodeHash,
		CodeExpiresAt: model.CodeExpiresAt, ExpiresAt: model.ExpiresAt, CreatedAt: model.CreatedAt,
	}
	if client != nil {
		mapped := clientToDomain(*client)
		entity.Client = &mapped
	}
	return entity
}

func requestFromDomain(entity domainoauth.AuthRequest) AuthRequest {
	return AuthRequest{
		ID: entity.ID, ClientID: entity.ClientID, RedirectURI: entity.RedirectURI, Scopes: slices.Clone(entity.Scopes),
		State: entity.State, CodeChallenge: entity.CodeChallenge, Resource: entity.Resource,
		Status: string(entity.Status), UserID: entity.UserID, GrantedScopes: slices.Clone(entity.GrantedScopes),
		CodeHash: entity.CodeHash, CodeExpiresAt: entity.CodeExpiresAt, ExpiresAt: entity.ExpiresAt, CreatedAt: entity.CreatedAt,
	}
}

func grantToDomain(model Grant, client *Client) domainoauth.Grant {
	entity := domainoauth.Grant{
		ID: model.ID, ClientID: model.ClientID, UserID: model.UserID, Scopes: slices.Clone([]string(model.Scopes)),
		CreatedAt: model.CreatedAt, LastUsedAt: model.LastUsedAt, RevokedAt: model.RevokedAt,
	}
	if client != nil {
		mapped := clientToDomain(*client)
		entity.Client = &mapped
	}
	return entity
}

func grantFromDomain(entity domainoauth.Grant) Grant {
	return Grant{
		ID: entity.ID, ClientID: entity.ClientID, UserID: entity.UserID, Scopes: slices.Clone(entity.Scopes),
		CreatedAt: entity.CreatedAt, LastUsedAt: entity.LastUsedAt, RevokedAt: entity.RevokedAt,
	}
}

func tokenToDomain(model Token) domainoauth.Token {
	return domainoauth.Token{
		ID: model.ID, GrantID: model.GrantID, Kind: domainoauth.TokenKind(model.Kind), TokenHash: model.TokenHash,
		ExpiresAt: model.ExpiresAt, RevokedAt: model.RevokedAt, CreatedAt: model.CreatedAt,
	}
}

func tokenFromDomain(entity domainoauth.Token) Token {
	return Token{
		ID: entity.ID, GrantID: entity.GrantID, Kind: string(entity.Kind), TokenHash: entity.TokenHash,
		ExpiresAt: entity.ExpiresAt, RevokedAt: entity.RevokedAt, CreatedAt: entity.CreatedAt,
	}
}
