package dto

import identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func FromUser(user *identitydomain.User) UserResponse {
	return UserResponse{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role}
}
