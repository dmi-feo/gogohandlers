package main

import (
	"encoding/json"
	ggh "gogohandlers"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlePing(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	sp, err := NewExampleAppServiceProvider("/tmp/test", logger)
	require.NoError(t, err)
	defer func() { _ = os.Remove("tmp/test") }()

	handler := ggh.Uitzicht[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandlePing,
		Middlewares: []func(func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, PingGetParams]) (*ggh.GGResponse[PingResponse, ExampleAppErrorData], error)) func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, PingGetParams]) (*ggh.GGResponse[PingResponse, ExampleAppErrorData], error){
			ggh.GetErrorHandlingMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData](HandleErrors),
			ggh.GetDataProcessingMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData](nil),
			ggh.RequestLoggingMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData],
			ggh.RequestIDMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData],
		},
		Logger: logger,
	}

	t.Run("works without get params", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/ping?mayfail=0", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)

		var responseBody PingResponse
		err = json.Unmarshal(response.Body.Bytes(), &responseBody)
		require.NoError(t, err)
		require.Equal(t, "pong", responseBody.Message)
	})

	t.Run("returns custom message", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/ping?mayfail=0&msg=test-message", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)

		var responseBody PingResponse
		err = json.Unmarshal(response.Body.Bytes(), &responseBody)
		require.NoError(t, err)
		require.Equal(t, "test-message", responseBody.Message)
	})

	t.Run("returns custom error data", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/ping?mustfail=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusTeapot, response.Code)

		var errorData ExampleAppErrorData
		err = json.Unmarshal(response.Body.Bytes(), &errorData)
		require.NoError(t, err)
		require.Equal(t, "TEAPOT", errorData.Code)
	})
}
