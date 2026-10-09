package handler

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/service"
)

type FrameHandler struct{ svc service.FrameService }

func NewFrameHandler(svc service.FrameService) *FrameHandler { return &FrameHandler{svc: svc} }
func (h *FrameHandler) Upload(c *gin.Context) {
	if c.Request.ContentLength > protocol.MaxFrameBodyBytes {
		writeError(c, &apperr.FrameTooLarge{Field: "body"})
		return
	}
	limited := http.MaxBytesReader(c.Writer, c.Request.Body, protocol.MaxFrameBodyBytes)
	defer limited.Close()
	raw, err := io.ReadAll(protocol.ContextReader{Context: c.Request.Context(), Reader: limited})
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeError(c, &apperr.FrameTooLarge{Field: "body"})
		} else {
			writeError(c, &apperr.InvalidParams{Field: "body"})
		}
		return
	}
	upload, err := protocol.ParseFrame(c.Request.Context(), c.GetHeader("Content-Type"), raw)
	if err != nil {
		writeError(c, err)
		return
	}
	result, created, err := h.svc.Save(c.Request.Context(), c.Param("device_id"), *upload)
	if err != nil {
		writeError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, Response{Code: apperr.CodeOK, Message: "ok", Data: result})
}
func (h *FrameHandler) Latest(c *gin.Context) {
	view, err := h.svc.Latest(c.Request.Context(), c.Param("device_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, view)
}
func (h *FrameHandler) Content(c *gin.Context) {
	f, err := h.svc.Open(c.Request.Context(), c.Param("snapshot_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	defer f.Close()
	// Read before writing headers so disk read failures retain the JSON error contract.
	raw, err := io.ReadAll(io.LimitReader(protocol.ContextReader{Context: c.Request.Context(), Reader: f}, protocol.MaxJPEGBytes+1))
	if err != nil {
		writeError(c, err)
		return
	}
	if len(raw) > protocol.MaxJPEGBytes {
		writeError(c, fmt.Errorf("stored snapshot exceeds byte limit"))
		return
	}
	c.Data(http.StatusOK, "image/jpeg", raw)
}
