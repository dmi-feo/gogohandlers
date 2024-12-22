package gogohandlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/gorilla/schema"
	"log/slog"
	"net/http"
	"time"
)

type MiddlewareProcessingError struct {
	Message    string
	StatusCode int
}

func (e MiddlewareProcessingError) Error() string {
	return e.Message
}

const (
	requestIDContextKey = "requestID"
)

type ServiceProvider interface{}

type GGRequest[TServiceProvider ServiceProvider, TReqBody, TGetParams any] struct {
	ServiceProvider *TServiceProvider
	RequestData     *TReqBody
	GetParams       *TGetParams
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
		ServiceProvider: u.ServiceProvider,
		RequestData:     nil,
		GetParams:       nil,
		Request:         r,
		Logger:          u.Logger,
	}

	theHandler := u.HandlerFunc

	for _, mw := range u.Middlewares {
		theHandler = mw(theHandler)
	}
	ggresp, handlerErr := theHandler(ggreq)

	statusCode := http.StatusOK // FIXME
	var responseData []byte

	if handlerErr != nil {
		ggreq.Logger.Warn("Handler returned uncaught error", slog.String("error", handlerErr.Error()))
		var mProcError MiddlewareProcessingError
		if errors.As(handlerErr, &mProcError) {
			statusCode = mProcError.StatusCode
			responseData = []byte(mProcError.Message)
		} else {
			panic(handlerErr) // FIXME
		}
	} else {
		if ggresp.ErrorOccured {
			if ggresp.StatusCode == 0 {
				statusCode = http.StatusInternalServerError
			} else {
				statusCode = ggresp.StatusCode
			}
		}
		responseData = ggresp.serializedResponse
	}

	for headerName, headerValues := range ggresp.Headers {
		for _, headerValue := range headerValues {
			w.Header().Set(headerName, headerValue)
		}
	}

	w.WriteHeader(statusCode)
	_, err := w.Write(responseData)
	if err != nil {
		panic(err) // FIXME
	}
}

func GetErrorHandlingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](errorHandlers ...func(err error, l *slog.Logger) (int, *TErrorData)) func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
			ggreq.Logger.Debug("ErrorHandlingMiddleware start")
			ggresp, err := hFunc(ggreq)
			if err != nil {
				ggreq.Logger.Warn("Going to handle error", slog.String("error", err.Error()))
				statusCode := http.StatusOK // FIXME
				var errorData *TErrorData
				for _, errorHandlerFunc := range errorHandlers {
					statusCode, errorData = errorHandlerFunc(err, ggreq.Logger)
					if statusCode != 0 {
						break
					}
				}
				if statusCode == 0 {
					return ggresp, err
				}

				ggresp.ErrorData = errorData
				ggresp.StatusCode = statusCode
				ggresp.ErrorOccured = true
			}

			ggreq.Logger.Debug("ErrorHandlingMiddleware finish")
			return ggresp, nil
		}
	}
}

type DataProcessingMiddlewareSettings struct {
	ForbidUnknownKeysInGetParams bool
}

func GetDataProcessingMiddleware[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any](settings *DataProcessingMiddlewareSettings) func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
	return func(hFunc func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
		return func(ggreq *GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error) {
			ggreq.Logger.Debug("DataProcessingMiddleware start")
			var reqBody TReqBody
			if ggreq.Request.Body != http.NoBody {
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
			if settings != nil {
				getParamsDecoder.IgnoreUnknownKeys(!settings.ForbidUnknownKeysInGetParams)
			}
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
