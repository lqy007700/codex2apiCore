package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetMergedUsageLogs(c *gin.Context) {
	start, e1 := time.Parse(time.RFC3339, c.Query("start"))
	end, e2 := time.Parse(time.RFC3339, c.Query("end"))
	if e1 != nil || e2 != nil || !end.After(start) {
		writeError(c, http.StatusBadRequest, "请提供有效的 start/end 时间范围")
		return
	}
	filter, ok := parseUsageLogsFilter(c, start, end)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	result, err := h.db.ListMergedUsageLogs(ctx, filter)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
