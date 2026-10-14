package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetUsageRequestChain(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "日志 ID 必须为正整数")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	chain, err := h.db.GetUsageRequestChain(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "日志不存在或已被清理，请刷新请求列表")
		return
	}
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, chain)
}
