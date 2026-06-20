package apperrors

import "net/http"

type Code string

const (
	CodeBadRequest   Code = "BAD_REQUEST"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"
	CodeNotFound     Code = "NOT_FOUND"
	CodeConflict     Code = "CONFLICT"
	CodeValidation   Code = "VALIDATION"
	CodeRateLimit    Code = "RATE_LIMIT"
	CodeInternal     Code = "INTERNAL"
)

type AppError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

func HTTPStatus(code Code) int {
	switch code {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeValidation:
		return http.StatusUnprocessableEntity
	case CodeRateLimit:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func New(code Code, message string) *AppError { return &AppError{Code: code, Message: message} }
func BadRequest(msg string) *AppError         { return New(CodeBadRequest, msg) }
func Unauthorized(msg string) *AppError       { return New(CodeUnauthorized, msg) }
func Forbidden(msg string) *AppError          { return New(CodeForbidden, msg) }
func NotFound(msg string) *AppError           { return New(CodeNotFound, msg) }
func Conflict(msg string) *AppError           { return New(CodeConflict, msg) }
func Validation(msg string) *AppError         { return New(CodeValidation, msg) }
func RateLimit(msg string) *AppError          { return New(CodeRateLimit, msg) }
func Internal(msg string) *AppError           { return New(CodeInternal, msg) }
