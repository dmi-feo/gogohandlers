package gogohandlers

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"slices"
	"time"
)

const (
	requestIDContextKey = "requestID"
)

type ServiceProvider interface{}

type GGRequest[TServiceProvider ServiceProvider, TReqBody, TGetParams any] struct {
	ServiceProvider TServiceProvider
	RequestData     TReqBody
	GetParams       TGetParams
	Request         *http.Request
	Logger          *slog.Logger
}

type GGResponse[TRespBody any] struct {
	ResponseData *TRespBody
	Headers      map[string][]string
}

// Waiting for https://github.com/golang/go/issues/68903
//type THandlerFunc[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any] = func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)
//type TMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any] = func(THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]

type Uitzicht[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any] struct {
	ServiceProvider *TServiceProvider
	HandlerFunc     func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)
	// Middlewares     []func(THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]
	Middlewares  []func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)
	ErrorHandler func(err error, l *slog.Logger) (int, TErrorData)
	Logger       *slog.Logger
}

func (u *Uitzicht[TServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var reqBody TReqBody
	if r.Body != http.NoBody {
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			slog.Info(
				"Error decoding request body",
				"error", err,
			)
			panic(err)
		}
	}

	ggreq := &GGRequest[TServiceProvider, TReqBody, TGetParams]{
		ServiceProvider: *u.ServiceProvider,
		RequestData:     reqBody,
		//GetParams:       nil,
		Request: r,
		Logger:  u.Logger,
	}

	theHandler := u.HandlerFunc
	for _, mw := range slices.Backward(u.Middlewares) {
		theHandler = mw(theHandler)
	}
	ggresp, handlerErr := theHandler(ggreq)

	statusCode := http.StatusOK // FIXME
	var errorData TErrorData
	if handlerErr != nil {
		statusCode, errorData = u.ErrorHandler(handlerErr, ggreq.Logger)
	}

	w.Header().Set("Content-Type", "application/json") // FIXME
	w.WriteHeader(statusCode)

	var bodySerialized []byte
	var serializationError error
	if handlerErr == nil {
		bodySerialized, serializationError = json.Marshal(ggresp.ResponseData)
	} else {
		bodySerialized, serializationError = json.Marshal(errorData)
	}
	if serializationError != nil {
		panic(serializationError)
	}

	for headerName, headerValues := range ggresp.Headers {
		for _, headerValue := range headerValues {
			w.Header().Add(headerName, headerValue)
		}
	}

	_, err := w.Write(bodySerialized)
	if err != nil {
		panic(err)
	}
}

// func RequestIDMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody] {
func RequestIDMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error) {
	return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error) {
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
		return ggresp, err
	}
}

// func RequestLoggingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody] {
func RequestLoggingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error) {
	return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error) {
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
		return ggresp, err
	}
}
