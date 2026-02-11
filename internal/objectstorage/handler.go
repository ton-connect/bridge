package objectstorage

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

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

type storeRequest struct {
	Object string `json:"object"`
}

type storeResponse struct {
	GetURL string `json:"get_url"`
}

type getResponse struct {
	Object string `json:"object"`
}

func (h *Handler) StoreHandler(c echo.Context) error {
	ttlStr := c.QueryParam("ttl")
	if ttlStr == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "missing ttl query parameter"})
	}

	ttl, err := strconv.ParseInt(ttlStr, 10, 64)
	if err != nil || ttl <= 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "ttl must be a positive integer"})
	}

	if ttl > h.maxTTL {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": fmt.Sprintf("ttl exceeds maximum allowed value of %d", h.maxTTL), "max_ttl": h.maxTTL})
	}

	var req storeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	if req.Object == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "missing object field"})
	}

	if len(req.Object) > h.maxSize {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": fmt.Sprintf("object exceeds maximum allowed size of %d bytes", h.maxSize)})
	}

	id, err := h.storage.Store(c.Request().Context(), req.Object, ttl)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to store object"})
	}

	getURL := h.buildGetURL(c, id)

	return c.JSON(http.StatusOK, storeResponse{GetURL: getURL})
}

func (h *Handler) GetHandler(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "missing id"})
	}

	object, err := h.storage.Get(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "object not found"})
	}

	return c.JSON(http.StatusOK, getResponse{Object: object})
}

func (h *Handler) buildGetURL(c echo.Context, id string) string {
	if h.baseURL != "" {
		return fmt.Sprintf("%s/store/%s", h.baseURL, id)
	}

	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	if fwd := c.Request().Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}

	return fmt.Sprintf("%s://%s/store/%s", scheme, c.Request().Host, id)
}
