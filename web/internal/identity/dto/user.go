package dto

import "github.com/RimuruChan/Vertex/web/internal/identity"

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func FromUser(user *identity.User) UserResponse {
	return UserResponse{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role}
}
