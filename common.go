package main

import "net/http"

type HttpRes struct {
	Message    string `json:"message,omitempty" example:"status ok"`
	StatusCode int    `json:"statusCode,omitempty" example:"200"`
}

func HttpResOk() HttpRes {
	return HttpRes{
		Message:    "OK",
		StatusCode: http.StatusOK,
	}
}

func HttpResError(errMsg string, statusCode int) (int, HttpRes) {
	return statusCode, HttpRes{
		Message:    errMsg,
		StatusCode: statusCode,
	}
}
