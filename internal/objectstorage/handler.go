package objectstorage

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

var allowedContentTypes = map[string]bool{
	"text/plain":       true,
	"application/json": true,
	"application/xml":  true,
}

type Handler struct {
	storage ObjectStorage
	maxTTL  int64
	maxSize int
	baseURL string
}

func NewHandler(storage ObjectStorage, maxTTL int64, maxSize int, baseURL string) *Handler {
	return &Handler{
		storage: storage,
		maxTTL:  maxTTL,
		maxSize: maxSize,
		baseURL: baseURL,
	}
}

func (h *Handler) StoreHandler(c echo.Context) error {
	ttlStr := c.QueryParam("ttl")
	if ttlStr == "" {
		return c.String(http.StatusBadRequest, "missing ttl query parameter")
	}

	ttl, err := strconv.ParseInt(ttlStr, 10, 64)
	if err != nil || ttl <= 0 {
		return c.String(http.StatusBadRequest, "ttl must be a positive integer")
	}

	if ttl > h.maxTTL {
		return c.String(http.StatusBadRequest, fmt.Sprintf("ttl exceeds maximum allowed value of %d", h.maxTTL))
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.String(http.StatusBadRequest, "failed to read request body")
	}

	if len(body) == 0 {
		return c.String(http.StatusBadRequest, "empty body")
	}

	if len(body) > h.maxSize {
		return c.String(http.StatusBadRequest, fmt.Sprintf("object exceeds maximum allowed size of %d bytes", h.maxSize))
	}

	id, err := h.storage.Store(c.Request().Context(), string(body), ttl)
	if err != nil {
		return c.String(http.StatusInternalServerError, "failed to store object")
	}

	getURL := h.buildGetURL(c, id)

	return c.String(http.StatusOK, getURL)
}

func (h *Handler) GetHandler(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.String(http.StatusBadRequest, "missing id")
	}

	object, err := h.storage.Get(c.Request().Context(), id)
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	// Use Accept header for content negotiation (RFC 7231), default to text/plain
	contentType := "text/plain"
	if accept := c.Request().Header.Get("Accept"); accept != "" && accept != "*/*" {
		if !allowedContentTypes[accept] {
			return c.String(http.StatusNotAcceptable, "unsupported Accept type")
		}
		contentType = accept
	}

	return c.Blob(http.StatusOK, contentType, []byte(object))
}

func (h *Handler) buildGetURL(c echo.Context, id string) string {
	if h.baseURL != "" {
		return fmt.Sprintf("%s/objects/%s", h.baseURL, id)
	}

	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	// Override scheme with X-Forwarded-Proto header set by reverse proxies (nginx, ALB, etc.)
	// to preserve the original protocol (e.g. https) when TLS is terminated at the proxy level.
	if fwd := c.Request().Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = strings.TrimSpace(fwd)
	}

	return fmt.Sprintf("%s://%s/objects/%s", scheme, strings.TrimSpace(c.Request().Host), id)
}
