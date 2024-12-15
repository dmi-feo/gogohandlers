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

type RequestSerializationError struct {
	ParserErrorMessage string
}

func (e RequestSerializationError) Error() string {
	return e.ParserErrorMessage
}

type ResponseSerializationError struct {
	ParserErrorMessage string
}

func (e ResponseSerializationError) Error() string {
	return e.ParserErrorMessage
}

const (
	requestIDContextKey = "requestID"
)

type ServiceProvider interface{}

type GGRequest[TServiceProvider ServiceProvider, TReqBody, TGetParams any] struct {
	ServiceProvider TServiceProvider
	RequestData     *TReqBody
	GetParams       TGetParams
	Request         *http.Request
	Logger          *slog.Logger
}

type GGResponse[TRespBody, TErrorData any] struct {
	ResponseData       *TRespBody
	ErrorOccured       bool
	ErrorData          *TErrorData
	StatusCode         int
	Headers            map[string][]string
	serializedResponse []byte
}

// Waiting for https://github.com/golang/go/issues/68903
//type THandlerFunc[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any] = func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (GGResponse[TRespBody], error)
//type TMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any] = func(THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]

type Uitzicht[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any] struct {
	ServiceProvider *TServiceProvider
	HandlerFunc     func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)
	// Middlewares     []func(THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]
	Middlewares []func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)
	Logger      *slog.Logger
}

func (u *Uitzicht[TServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ggreq := &GGRequest[TServiceProvider, TReqBody, TGetParams]{
		ServiceProvider: *u.ServiceProvider,
		RequestData:     nil,
		//GetParams:       nil,
		Request: r,
		Logger:  u.Logger,
	}

	theHandler := u.HandlerFunc

	for _, mw := range slices.Backward(u.Middlewares) {
		theHandler = mw(theHandler)
	}
	ggresp, handlerErr := theHandler(ggreq)
	if handlerErr != nil {
		panic(handlerErr)
	}

	statusCode := http.StatusOK // FIXME
	if ggresp.ErrorOccured {
		if ggresp.StatusCode == 0 {
			statusCode = http.StatusInternalServerError
		} else {
			statusCode = ggresp.StatusCode
		}
	}

	for headerName, headerValues := range ggresp.Headers {
		for _, headerValue := range headerValues {
			w.Header().Set(headerName, headerValue)
		}
	}

	w.WriteHeader(statusCode)

	_, err := w.Write(ggresp.serializedResponse)
	if err != nil {
		panic(err)
	}
}

func GetErrorHandlingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](errorHandlers ...func(err error, l *slog.Logger) (int, *TErrorData)) func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
			ggresp, err := hFunc(ggreq)
			if err != nil {
				statusCode := http.StatusOK // FIXME
				var errorData *TErrorData
				for _, errorHandlerFunc := range errorHandlers {
					statusCode, errorData = errorHandlerFunc(err, ggreq.Logger)
					if statusCode != 0 {
						break
					}
				}
				if statusCode == 0 {
					statusCode = http.StatusInternalServerError
				}

				ggresp.ErrorData = errorData
				ggresp.StatusCode = statusCode
				ggresp.ErrorOccured = true
			}

			return ggresp, nil
		}
	}
}

func GetDataProcessingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any]() func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
			var reqBody TReqBody
			if ggreq.Request.Body != http.NoBody {
				err := json.NewDecoder(ggreq.Request.Body).Decode(&reqBody)
				if err != nil {
					slog.Info(
						"Error decoding request body",
						"error", err,
					)
					return nil, RequestSerializationError{ParserErrorMessage: err.Error()}
				}
			}
			ggreq.RequestData = &reqBody

			ggresp, err := hFunc(ggreq)
			if err != nil {
				panic(err)
			}

			var bodySerialized []byte
			var serializationError error

			if !ggresp.ErrorOccured {
				bodySerialized, serializationError = json.Marshal(ggresp.ResponseData)
			} else {
				bodySerialized, serializationError = json.Marshal(ggresp.ErrorData)
			}
			if serializationError != nil {
				return ggresp, RequestSerializationError{ParserErrorMessage: serializationError.Error()}
			}
			ggresp.serializedResponse = bodySerialized
			if ggresp.Headers == nil {
				ggresp.Headers = make(map[string][]string)
			}
			ggresp.Headers["content-type"] = []string{"application/json"}

			return ggresp, err
		}
	}
}

// func RequestIDMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody any](hFunc THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody] {
func RequestIDMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
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
func RequestLoggingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
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
