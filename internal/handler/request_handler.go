package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"rate-limited-api/internal/model"
	"rate-limited-api/internal/service"
	"rate-limited-api/internal/worker"
)

type RequestHandler struct {
	service     *service.RequestService
	retryWorker *worker.RetryWorker
}

func NewRequestHandler(svc *service.RequestService, rw *worker.RetryWorker) *RequestHandler {
	return &RequestHandler{
		service:     svc,
		retryWorker: rw,
	}
}

func (h *RequestHandler) Handle(c *gin.Context) {
	var req model.RequestPayload

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body: " + err.Error(),
		})
		return
	}

	allowed, retryAfter, err := h.service.ProcessRequest(c.Request.Context(), req.UserID, req.Payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	if !allowed {
		if err := h.retryWorker.EnqueueRetry(c.Request.Context(), req.UserID, req.Payload, retryAfter); err != nil {
			log.Printf("Failed to enqueue retry for user %s: %v", req.UserID, err)
		}

		c.JSON(http.StatusTooManyRequests, model.ErrorResponse{
			Error:      "rate limit exceeded",
			RetryAfter: retryAfter,
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Status: "success",
	})
}
