package gogohandlers

import (
	"bytes"
	"encoding/json"
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

		request, err := http.NewRequest(http.MethodGet, "/?param_str=hello&param_bool=0", nil)
		require.NoError(t, err)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("request body", func(t *testing.T) {
		type NestedSchema struct {
			StringField string `json:"field_str"`
		}
		type RequestBodySchema struct {
			StringField string       `json:"field_str"`
			IntField    int          `json:"field_int"`
			FloatField  float64      `json:"field_float"`
			BoolField   bool         `json:"field_bool"`
			NestedField NestedSchema `json:"nested_field"`
		}

		reqBody := RequestBodySchema{
			StringField: "hello",
			IntField:    13,
			FloatField:  0.1,
			BoolField:   true,
			NestedField: NestedSchema{
				StringField: "world",
			},
		}

		var handlerFunc = func(ggreq *GGRequest[struct{}, RequestBodySchema, struct{}]) (*GGResponse[struct{}, struct{}], error) {
			if *ggreq.RequestData != reqBody {
				return &GGResponse[struct{}, struct{}]{}, fmt.Errorf("request_body has unexpected value: %+v", ggreq.RequestData)
			}
			return &GGResponse[struct{}, struct{}]{}, nil
		}

		handler := GGHandler[struct{}, RequestBodySchema, struct{}, struct{}, struct{}]{
			ServiceProvider: &struct{}{},
			HandlerFunc:     handlerFunc,
			Middlewares: []func(hFunc func(*GGRequest[struct{}, RequestBodySchema, struct{}]) (*GGResponse[struct{}, struct{}], error)) func(*GGRequest[struct{}, RequestBodySchema, struct{}]) (*GGResponse[struct{}, struct{}], error){
				GetDataProcessingMiddleware[struct{}, RequestBodySchema, struct{}, struct{}, struct{}](nil),
			},
			Logger: logger,
		}

		bodySerialized, err := json.Marshal(reqBody)
		require.NoError(t, err)
		request, err := http.NewRequest(http.MethodGet, "/", bytes.NewReader(bodySerialized))
		require.NoError(t, err)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)
	})
}
