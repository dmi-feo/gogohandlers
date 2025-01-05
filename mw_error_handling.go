package gogohandlers

import (
	"log/slog"
	"net/http"
)

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
