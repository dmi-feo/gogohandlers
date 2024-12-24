package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
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

	dbFilePath := fmt.Sprintf("/tmp/test_%s", uuid.New().String())
	defer func() { _ = os.Remove(dbFilePath) }()

	sp, err := NewExampleAppServiceProvider(dbFilePath, logger)
	require.NoError(t, err)

	handler := ggh.Uitzicht[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandlePing,
		Middlewares:     getDefaultMiddlewares[struct{}, PingGetParams, PingResponse](),
		Logger:          logger,
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

func TestHandleGetSetValue(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dbFilePath := fmt.Sprintf("/tmp/test_%s", uuid.New().String())
	defer func() { _ = os.Remove(dbFilePath) }()

	sp, err := NewExampleAppServiceProvider(dbFilePath, logger)
	require.NoError(t, err)

	setValueHandler := ggh.Uitzicht[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandleSetValue,
		Middlewares:     getDefaultMiddlewares[SetValueRequest, struct{}, SetValueResponse](),
		Logger:          logger,
	}

	getValueHandler := ggh.Uitzicht[ExampleAppServiceProvider, struct{}, struct{}, GetValueResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandleGetValue,
		Middlewares:     getDefaultMiddlewares[struct{}, struct{}, GetValueResponse](),
		Logger:          logger,
	}

	t.Run("set and get value", func(t *testing.T) {
		reqData := SetValueRequest{Key: "test-key", Value: "test-value"}
		reqBody, err := json.Marshal(reqData)
		require.NoError(t, err)
		request, err := http.NewRequest(http.MethodPost, "/set_value", bytes.NewReader(reqBody))
		require.NoError(t, err)
		response := httptest.NewRecorder()

		setValueHandler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)

		request, err = http.NewRequest(http.MethodGet, "/get_value/{key}", nil)
		require.NoError(t, err)
		request.SetPathValue("key", "test-key")
		response = httptest.NewRecorder()

		getValueHandler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var responseBody GetValueResponse
		err = json.Unmarshal(response.Body.Bytes(), &responseBody)
		require.NoError(t, err)
		require.Equal(t, "test-value", responseBody.Value)
	})
}
