package gogohandlers

import (
	"context"

	"github.com/google/uuid"
)

const (
	requestIDContextKey = "requestID"
)

// func RequestIDMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody] {
func RequestIDMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		ggreq.Logger.Debug("RequestIDMiddleware start")
		var requestID string
		if requestIDHeader, ok := ggreq.Request.Header["X-Request-Id"]; ok {
			requestID = requestIDHeader[0]
		} else {
			requestID = uuid.New().String()
		}
		ggreq.Request = ggreq.Request.WithContext(context.WithValue(ggreq.Request.Context(), requestIDContextKey, requestID))
		ggresp, err := hFunc(ggreq)

		if ggresp.Headers == nil {
			ggresp.Headers = make(map[string][]string)
		}
		ggresp.Headers["X-Request-Id"] = []string{requestID}
		ggreq.Logger.Debug("RequestIDMiddleware finish")
		return ggresp, err
	}
}
