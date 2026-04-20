package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"rate-limited-api/internal/model"
	"rate-limited-api/internal/service"
)

type StatsHandler struct {
	service *service.RequestService
}

func NewStatsHandler(svc *service.RequestService) *StatsHandler {
	return &StatsHandler{service: svc}
}

func (h *StatsHandler) Handle(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "missing required query parameter: user_id",
		})
		return
	}

	stats, err := h.service.GetStats(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "failed to retrieve stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
