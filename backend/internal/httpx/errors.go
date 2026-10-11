package httpx

import "github.com/gin-gonic/gin"

// ErrorResponse is the stable error envelope used across the API.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Fail writes a stable error response and aborts the handler chain.
func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message}})
}

// Common error codes used across handlers.
const (
	CodeInvalidRequest    = "INVALID_REQUEST"
	CodeUnauthorized      = "UNAUTHORIZED"
	CodeForbidden         = "FORBIDDEN"
	CodeCSRF              = "CSRF_FAILED"
	CodeRateLimited       = "RATE_LIMITED"
	CodeNotFound          = "NOT_FOUND"
	CodeHandleNotFound    = "HANDLE_NOT_FOUND"
	CodeHandleTaken       = "HANDLE_TAKEN"
	CodeHandleInvalid     = "HANDLE_INVALID"
	CodeEmailTaken        = "EMAIL_TAKEN"
	CodeInvalidCredential = "INVALID_CREDENTIALS"
	CodeConflict          = "CONFLICT"
	CodeInternal          = "INTERNAL_ERROR"
	CodeWalletInvalid     = "WALLET_INVALID"
	CodeChallengeInvalid  = "CHALLENGE_INVALID"
	CodeTxInvalid         = "TRANSACTION_INVALID"
	CodeProviderDisabled  = "PROVIDER_NOT_CONFIGURED"
)
