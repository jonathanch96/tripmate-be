package invitationrequest

type Create struct {
	Email string `json:"email" binding:"required,email,max=255"`
	Name  string `json:"name" binding:"max=160"`
	// Password is only used when the email has no account yet - the trip owner sets it for the
	// new member right here (they can also sign in with Google SSO using this email instead).
	// Ignored when the email already belongs to an existing account.
	Password string `json:"password" binding:"max=72"`
}
