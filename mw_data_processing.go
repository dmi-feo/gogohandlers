package gogohandlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/schema"
)

type DataProcessingMiddlewareSettings struct {
	ForbidUnknownKeysInGetParams bool
}

func GetDataProcessingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](settings *DataProcessingMiddlewareSettings) func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
			ggreq.Logger.Debug("DataProcessingMiddleware start")
			if settings == nil {
				settings = &DataProcessingMiddlewareSettings{}
			}

			var reqBody TReqBody
			if ggreq.Request.Body != http.NoBody && ggreq.Request.Body != nil {
				err := json.NewDecoder(ggreq.Request.Body).Decode(&reqBody)
				if err != nil {
					slog.Info(
						"Error decoding request body",
						"error", err,
					)
					return nil, MiddlewareProcessingError{Message: err.Error(), StatusCode: http.StatusBadRequest}
				}
			}
			ggreq.RequestData = &reqBody

			getParamsDecoder := schema.NewDecoder()
			getParamsDecoder.IgnoreUnknownKeys(!settings.ForbidUnknownKeysInGetParams)
			var getParams TGetParams
			err := getParamsDecoder.Decode(&getParams, ggreq.Request.URL.Query())
			if err != nil {
				return &GGResponse[TRespBody, TErrorData]{}, MiddlewareProcessingError{Message: err.Error(), StatusCode: http.StatusBadRequest}
			}
			ggreq.GetParams = &getParams

			ggresp, err := hFunc(ggreq)
			if err != nil {
				return &GGResponse[TRespBody, TErrorData]{}, err
			}

			var bodySerialized []byte
			var serializationError error

			if !ggresp.ErrorOccured {
				bodySerialized, serializationError = json.Marshal(ggresp.ResponseData)
			} else {
				bodySerialized, serializationError = json.Marshal(ggresp.ErrorData)
			}
			if serializationError != nil {
				return ggresp, MiddlewareProcessingError{Message: serializationError.Error(), StatusCode: http.StatusBadRequest}
			}
			ggresp.serializedResponse = bodySerialized
			if ggresp.Headers == nil {
				ggresp.Headers = make(map[string][]string)
			}
			ggresp.Headers["content-type"] = []string{"application/json"}

			ggreq.Logger.Debug("DataProcessingMiddleware finish")
			return ggresp, err
		}
	}
}
