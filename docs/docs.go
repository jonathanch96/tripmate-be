package docs

import "github.com/swaggo/swag"

const docTemplate = `{"swagger":"2.0","info":{"title":"TripMate API","description":"Shared-trip expense settlement API.","version":"1.0"},"host":"localhost:8080","basePath":"/api/v1","paths":{}}`

type swaggerInfo struct{}

func (swaggerInfo) ReadDoc() string { return docTemplate }

func init() { swag.Register("swagger", swaggerInfo{}) }
