package fxrequest

type SetRate struct {
	From string `json:"from" binding:"required,len=3"`
	To   string `json:"to" binding:"required,len=3"`
	Rate string `json:"rate" binding:"required"`
}
