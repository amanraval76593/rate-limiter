package model

type RequestPayload struct {
	UserID  string                 `json:"user_id" binding:"required"`
	Payload map[string]interface{} `json:"payload" binding:"required"`
}

type SuccessResponse struct {
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error      string `json:"error"`
	RetryAfter int64  `json:"retry_after,omitempty"`
}

type StatsResponse struct {
	UserID        string `json:"user_id"`
	TotalRequests int64  `json:"total_requests"`
	Allowed       int64  `json:"allowed"`
	Blocked       int64  `json:"blocked"`
}
