package gogohandlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataProcessingMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("get params", func(t *testing.T) {
		type GetParams struct {
			ParamString string `schema:"param_str"`
			ParamInt    *int   `schema:"param_int,default:13"`
			ParamBool   *bool  `schema:"param_bool,default:true"`
		}

		var handlerFunc = func(ggreq *GGRequest[struct{}, struct{}, GetParams]) (*GGResponse[struct{}, struct{}], error) {
			if ggreq.GetParams.ParamString != "hello" {
				return nil, fmt.Errorf("param_str has unexpected value: %s", ggreq.GetParams.ParamString)
			}
			if *ggreq.GetParams.ParamInt != 13 {
				return nil, fmt.Errorf("param_int has unexpected value: %d", *ggreq.GetParams.ParamInt)
			}
			if *ggreq.GetParams.ParamBool {
				return nil, fmt.Errorf("param_bool has unexpected value: %t", *ggreq.GetParams.ParamBool)
			}
			return &GGResponse[struct{}, struct{}]{}, nil
		}

		handler := GGHandler[struct{}, struct{}, GetParams, struct{}, struct{}]{
			ServiceProvider: &struct{}{},
			HandlerFunc:     handlerFunc,
			Middlewares: []func(hFunc func(*GGRequest[struct{}, struct{}, GetParams]) (*GGResponse[struct{}, struct{}], error)) func(*GGRequest[struct{}, struct{}, GetParams]) (*GGResponse[struct{}, struct{}], error){
				GetDataProcessingMiddleware[struct{}, struct{}, GetParams, struct{}, struct{}](nil),
			},
			Logger: logger,
		}

		request, err := http.NewRequest(http.MethodGet, "/?param_str=hello&param_bool=false", nil)
		require.NoError(t, err)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)
	})
}
