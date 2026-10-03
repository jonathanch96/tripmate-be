package oauth

import oauthdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/oauth"

var _ oauthdomain.Repository = (*adapterGormPostgresql)(nil)
