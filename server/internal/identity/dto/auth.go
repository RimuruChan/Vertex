package dto

import "github.com/RimuruChan/Vertex/server/internal/identity"

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	AccessToken string       `json:"accessToken"`
	Token       string       `json:"token"` // Deprecated: use accessToken.
	ExpiresIn   int64        `json:"expiresIn"`
	User        UserResponse `json:"user"`
}

func FromResult(result *identity.Result) AuthResponse {
	return AuthResponse{
		AccessToken: result.AccessToken,
		Token:       result.AccessToken,
		ExpiresIn:   result.ExpiresIn,
		User:        FromUser(result.User),
	}
}
