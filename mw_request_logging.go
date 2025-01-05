package gogohandlers

import (
	"log/slog"
	"time"
)

// func RequestLoggingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody] {
func RequestLoggingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		ggreq.Logger.Debug("RequestLoggingMiddleware start")
		reqIDValue := ggreq.Request.Context().Value(requestIDContextKey)
		var requestID string
		if reqIDValue != nil {
			requestID = reqIDValue.(string)
		}
		ggreq.Logger = ggreq.Logger.With(
			slog.String("request_id", requestID),
		)

		ggreq.Logger.Info(
			"New request",
			slog.String("method", ggreq.Request.Method),
			slog.String("url", ggreq.Request.URL.String()),
		)
		start := time.Now()
		ggresp, err := hFunc(ggreq)
		elapsed := time.Since(start)
		ggreq.Logger.Info(
			"Request finished",
			slog.String("method", ggreq.Request.Method),
			slog.String("url", ggreq.Request.URL.String()),
			slog.Duration("duration", elapsed),
		)
		ggreq.Logger.Debug("RequestLoggingMiddleware finish")
		return ggresp, err
	}
}
