package dto

type ContestRegistrationRequest struct {
	Password string `json:"password,omitempty"`
}

type RegistrationResponse struct {
	Registered bool `json:"registered"`
}
