package expensecategoryrequest

type Create struct {
	Name string `json:"name" binding:"required,max=50"`
}
