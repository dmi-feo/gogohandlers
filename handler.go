package gogohandlers

import (
	"errors"
	"log/slog"
	"net/http"
)

type MiddlewareProcessingError struct {
	Message    string
	StatusCode int
}

func (e MiddlewareProcessingError) Error() string {
	return e.Message
}

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

type GGHandler[TServiceProvider ServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData any] struct {
	ServiceProvider *TServiceProvider
	HandlerFunc     func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)
	// Middlewares     []func(THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]) THandlerFunc[TServiceProvider, TReqBody, TGetParams, TRespBody]
	Middlewares []func(func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)) func(*GGRequest[TServiceProvider, TReqBody, TGetParams]) (*GGResponse[TRespBody, TErrorData], error)
	Logger      *slog.Logger
}

func (u *GGHandler[TServiceProvider, TReqBody, TGetParams, TRespBody, TErrorData]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
			statusCode = http.StatusInternalServerError
		}
	} else {
		responseData = ggresp.serializedResponse
		if ggresp.StatusCode == 0 {
			if ggresp.ErrorOccured {
				statusCode = http.StatusInternalServerError
			} else {
				statusCode = http.StatusOK
			}
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
	_, err := w.Write(responseData)
	if err != nil {
		u.Logger.Warn("Failed to write response", slog.String("error", err.Error()))
	}
}
