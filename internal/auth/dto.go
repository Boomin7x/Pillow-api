package auth

import "github.com/kodiahbertrand/pillow/internal/domain"

type RegisterRequest struct {
	Email       string `json:"email"        validate:"required,email"`
	Password    string `json:"password"     validate:"required,min=8"`
	DisplayName string `json:"display_name" validate:"required,min=2,max=100"`
}

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type RefreshRequest struct {
	// Refresh token is read from the httpOnly cookie, not the body.
	// This struct is intentionally empty but kept for consistency.
}

type AuthResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	User        UserPayload `json:"user"`
}

type UserPayload struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func authResponseFromDomain(result *domain.AuthResult) AuthResponse {
	return AuthResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		User: UserPayload{
			ID:          result.User.ID,
			Email:       result.User.Email,
			DisplayName: result.User.DisplayName,
		},
	}
}
