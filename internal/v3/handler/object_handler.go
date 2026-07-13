package handlerv3

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
)

// allowedContentTypes is the whitelist of Content-Type values accepted for stored objects.
var allowedContentTypes = map[string]bool{
	"text/plain":       true,
	"application/json": true,
	"application/xml":  true,
}

// ObjectHandler provides HTTP endpoints for storing and retrieving objects.
type ObjectHandler struct {
	storage storagev3.Storage
	maxTTL  int64
	maxSize int64
	baseURL string
}

// NewObjectHandler creates an ObjectHandler with the given storage backend, max TTL (seconds),
// max object size (bytes), and optional base URL for generating object retrieval links.
func NewObjectHandler(storage storagev3.Storage, maxTTL int64, maxSize int64, baseURL string) *ObjectHandler {
	return &ObjectHandler{
		storage: storage,
		maxTTL:  maxTTL,
		maxSize: maxSize,
		baseURL: baseURL,
	}
}

// StoreHandler handles POST /objects. It validates the TTL query parameter, Content-Type header,
// and body size, then stores the object and returns the retrieval URL.
func (h *ObjectHandler) StoreHandler(c echo.Context) error {
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

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, h.maxSize+1))
	if err != nil {
		return c.String(http.StatusBadRequest, "failed to read request body")
	}

	if len(body) == 0 {
		return c.String(http.StatusBadRequest, "empty body")
	}

	if int64(len(body)) > h.maxSize {
		return c.String(http.StatusBadRequest, fmt.Sprintf("object exceeds maximum allowed size of %d bytes", h.maxSize))
	}

	contentType := c.Request().Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}
	if !allowedContentTypes[contentType] {
		return c.String(http.StatusBadRequest, "unsupported Content-Type")
	}

	id, err := h.storage.StoreObject(c.Request().Context(), body, contentType, ttl)
	if err != nil {
		return c.String(http.StatusInternalServerError, "failed to store object")
	}

	getURL := h.buildGetURL(c, id)

	return c.String(http.StatusOK, getURL)
}

// GetHandler handles GET /objects/:id. It retrieves the object by ID and returns it
// with the original Content-Type.
func (h *ObjectHandler) GetHandler(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.String(http.StatusBadRequest, "missing id")
	}

	object, contentType, err := h.storage.GetObject(c.Request().Context(), id)
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	return c.Blob(http.StatusOK, contentType, object)
}

// buildGetURL constructs the full URL for retrieving a stored object.
// Uses baseURL if configured, otherwise derives scheme and host from the request,
// respecting X-Forwarded-Proto for TLS termination at a reverse proxy.
func (h *ObjectHandler) buildGetURL(c echo.Context, id string) string {
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
