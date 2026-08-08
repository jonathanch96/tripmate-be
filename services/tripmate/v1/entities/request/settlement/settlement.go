package settlementrequest

type Create struct {
	FromUserID string  `json:"from_user_id" binding:"required,uuid"`
	ToUserID   string  `json:"to_user_id" binding:"required,uuid"`
	Amount     string  `json:"amount" binding:"required"`
	Currency   string  `json:"currency" binding:"required,len=3"`
	Method     string  `json:"method" binding:"required,oneof=cash bank_transfer"`
	Note       *string `json:"note"`
	ProofURL   *string `json:"proof_url"`
}

type Reject struct {
	Reason string `json:"reason" binding:"required,max=500"`
}
