package invitationrequest

type Create struct {
	Email string `json:"email" binding:"required,email,max=255"`
}
