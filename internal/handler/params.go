package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/labstack/echo/v4"
)

type ParamsStorage struct {
	params map[string]string
}

func NewParamsStorage(c echo.Context, maxBodySize int64) (*ParamsStorage, error) {
	ps := &ParamsStorage{
		params: make(map[string]string),
	}

	bodyContent := []byte{}
	var contentType string
	if c.Request().Body != nil {
		limitedReader := io.LimitReader(c.Request().Body, maxBodySize+1)
		bodyBytes, err := io.ReadAll(limitedReader)
		if err != nil {
			return nil, err
		}
		if int64(len(bodyBytes)) > maxBodySize {
			return nil, fmt.Errorf("request body too large")
		}
		bodyContent = bodyBytes
		c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
		contentType = c.Request().Header.Get("Content-Type")
	}

	bodyParams := ps.parseBodyParams(bodyContent, contentType)
	if len(bodyParams) > 0 {
		ps.params = bodyParams
	} else {
		ps.params = ps.parseURLParams(c)
	}

	return ps, nil
}

func (p *ParamsStorage) Get(key string) (string, bool) {
	val, ok := p.params[key]
	return val, ok && len(val) > 0
}

func (p *ParamsStorage) parseBodyParams(bodyContent []byte, contentType string) map[string]string {
	bodyParams := make(map[string]string)

	if len(bodyContent) > 0 && strings.Contains(contentType, "application/json") {
		var parsed map[string]interface{}
		if json.Unmarshal(bodyContent, &parsed) == nil {
			for key, val := range parsed {
				if str, ok := val.(string); ok {
					bodyParams[key] = str
				}
			}
		}
	}

	return bodyParams
}

// TODO if you need to retrieve list of values for a key, you have to support it
func (p *ParamsStorage) parseURLParams(c echo.Context) map[string]string {
	urlParams := make(map[string]string)
	for key, values := range c.QueryParams() {
		if len(values) > 0 && values[0] != "" {
			urlParams[key] = values[0]
		}
	}
	return urlParams
}
