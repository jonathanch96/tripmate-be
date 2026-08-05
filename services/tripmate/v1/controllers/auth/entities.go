package auth

import userdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/user"

type controller struct{ users userdomain.Service }

func NewController(users userdomain.Service) Controller { return &controller{users: users} }
