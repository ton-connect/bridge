package main

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/labstack/echo/v4"
)

type ParamsStorage struct {
	params      map[string]string
	bodyContent []byte // Store original body content
}

func NewParamsStorage(c echo.Context) *ParamsStorage {
	ps := &ParamsStorage{
		params: make(map[string]string),
	}

	// Always read body first and store it
	if c.Request().Body != nil {
		bodyBytes, err := io.ReadAll(c.Request().Body)
		if err == nil {
			ps.bodyContent = bodyBytes
			// TODO what if not restore it?
			c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	// Parse and store all parameters
	bodyParams := ps.parseBodyParams()
	if len(bodyParams) > 0 {
		ps.params = bodyParams
	} else {
		ps.params = ps.parseURLParams(c)
	}

	return ps
}

func (p *ParamsStorage) Get(key string) (string, bool) {
	val, ok := p.params[key]
	return val, ok
}

// GetMessageContent returns the actual message content for SendMessageHandler
func (p *ParamsStorage) GetMessageContent() []byte {
	return p.bodyContent
}

func (p *ParamsStorage) parseBodyParams() map[string]string {
	bodyParams := make(map[string]string)

	if len(p.bodyContent) > 0 {
		var parsed map[string]interface{}
		if json.Unmarshal(p.bodyContent, &parsed) == nil {
			for key, val := range parsed {
				if str, ok := val.(string); ok {
					bodyParams[key] = str
				}
			}
		}
	}

	return bodyParams
}

func (p *ParamsStorage) parseURLParams(c echo.Context) map[string]string {
	urlParams := make(map[string]string)
	for key, values := range c.QueryParams() {
		if len(values) > 0 && values[0] != "" {
			urlParams[key] = values[0]
		}
	}
	return urlParams
}
