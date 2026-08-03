package httpx

import "github.com/gin-gonic/gin"

type StatusResponse struct {
	Status string `json:"status"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

type ErrorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func WriteError(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{Code: code, Error: message})
}
