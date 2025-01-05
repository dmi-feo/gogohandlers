package gogohandlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGGHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("dummy", func(t *testing.T) {
		var handlerFunc = func(ggreq *GGRequest[struct{}, struct{}, struct{}]) (*GGResponse[struct{}, struct{}], error) {
			return &GGResponse[struct{}, struct{}]{}, nil
		}

		handler := GGHandler[struct{}, struct{}, struct{}, struct{}, struct{}]{
			ServiceProvider: &struct{}{},
			HandlerFunc:     handlerFunc,
			Middlewares:     []func(hFunc func(*GGRequest[struct{}, struct{}, struct{}]) (*GGResponse[struct{}, struct{}], error)) func(*GGRequest[struct{}, struct{}, struct{}]) (*GGResponse[struct{}, struct{}], error){},
			Logger:          logger,
		}

		request, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)
	})
}
