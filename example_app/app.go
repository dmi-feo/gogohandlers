package main

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"
	ggh "gogohandlers"
)

type ExampleAppErrorData struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details"`
}

func HandleErrors(err error, l *slog.Logger) (statusCode int, errorData *ExampleAppErrorData) {
	l.Warn("Handling error", slog.String("error", err.Error()))
	switch err.(type) {
	case RandomError:
		statusCode, errorData = 418, &ExampleAppErrorData{Code: "TEAPOT", Message: err.Error(), Details: map[string]string{"reason": "destiny"}}
	case DatabaseError:
		statusCode, errorData = 424, &ExampleAppErrorData{Code: "DATABASE", Message: err.Error(), Details: nil}
	}
	if statusCode != 0 {
		l.Warn("Handled error", slog.Int("status_code", statusCode), slog.String("code", errorData.Code))
	}
	return
}

type RandomError struct{}

func (err RandomError) Error() string {
	return "Random error"
}

type DatabaseError struct {
	DBMessage string
}

func (err DatabaseError) Error() string {
	return fmt.Sprintf("Database error: %s", err.DBMessage)
}

type TheStorage struct {
	logger   *slog.Logger
	filePath string
}

func NewTheStorage(filePath string, logger *slog.Logger) (*TheStorage, error) {
	db, err := sql.Open("sqlite3", filePath)
	defer db.Close()
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS storage (key string NOT NULL PRIMARY KEY, value string)`)
	if err != nil {
		return nil, err
	}
	return &TheStorage{filePath: filePath, logger: logger}, nil
}

func (ts *TheStorage) getDb() (*sql.DB, error) {
	return sql.Open("sqlite3", ts.filePath)
}

func (ts *TheStorage) Get(key string) (*string, error) {
	db, err := ts.getDb()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT value FROM storage WHERE key = ?`, key)
	if err != nil {
		return nil, err
	}
	res := rows.Next()
	if !res {
		return nil, nil
	}
	var value string
	err = rows.Scan(&value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (ts *TheStorage) Set(key string, value string) error {
	ts.logger.Info("Setting key", slog.String("key", key))
	db, err := ts.getDb()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO storage (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		return err
	}
	return nil
}

type ExampleAppServiceProvider struct {
	logger  *slog.Logger
	storage *TheStorage
}

func NewExampleAppServiceProvider(filePath string, logger *slog.Logger) (*ExampleAppServiceProvider, error) {
	easp := &ExampleAppServiceProvider{logger: logger}
	var err error
	easp.storage, err = NewTheStorage(filePath, logger)
	if err != nil {
		return nil, err
	}
	return easp, nil
}

func (sp *ExampleAppServiceProvider) GetStorage() *TheStorage {
	return sp.storage
}

type PingGetParams struct {
	Message  string `schema:"msg,default:pong"`
	MayFail  bool   `schema:"mayfail"`
	MustFail bool   `schema:"mustfail"`
}

type PingResponse struct {
	Message string `json:"msg"`
}

func HandlePing(ggreq *ggh.GGRequest[ExampleAppServiceProvider, struct{}, PingGetParams]) (*ggh.GGResponse[PingResponse, ExampleAppErrorData], error) {
	ggreq.Logger.Info("Preparing pong...")
	if ggreq.GetParams.MayFail && rand.Intn(2) == 1 || ggreq.GetParams.MustFail {
		return &ggh.GGResponse[PingResponse, ExampleAppErrorData]{}, RandomError{}
	}
	return &ggh.GGResponse[PingResponse, ExampleAppErrorData]{
		ResponseData: &PingResponse{
			Message: ggreq.GetParams.Message,
		},
	}, nil
}

type SetValueRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SetValueResponse struct {
	Message string `json:"message"`
}

func HandleSetValue(ggreq *ggh.GGRequest[ExampleAppServiceProvider, SetValueRequest, struct{}]) (*ggh.GGResponse[SetValueResponse, ExampleAppErrorData], error) {
	storage := ggreq.ServiceProvider.GetStorage()
	err := storage.Set(ggreq.RequestData.Key, ggreq.RequestData.Value)
	if err != nil {
		return &ggh.GGResponse[SetValueResponse, ExampleAppErrorData]{}, DatabaseError{DBMessage: err.Error()}
	}
	return &ggh.GGResponse[SetValueResponse, ExampleAppErrorData]{
		ResponseData: &SetValueResponse{Message: "ok"},
	}, nil
}

type GetValueResponse struct {
	Value string `json:"value"`
}

func HandleGetValue(ggreq *ggh.GGRequest[ExampleAppServiceProvider, struct{}, struct{}]) (*ggh.GGResponse[GetValueResponse, ExampleAppErrorData], error) {
	key := ggreq.Request.PathValue("key")
	storage := ggreq.ServiceProvider.GetStorage()
	value, err := storage.Get(key)
	if err != nil {
		return &ggh.GGResponse[GetValueResponse, ExampleAppErrorData]{}, DatabaseError{DBMessage: err.Error()}
	}
	return &ggh.GGResponse[GetValueResponse, ExampleAppErrorData]{
		ResponseData: &GetValueResponse{Value: *value},
	}, nil
}

func main() {
	loggingHandler := slog.NewJSONHandler(os.Stdout, nil)
	logger := slog.New(loggingHandler)

	mux := http.NewServeMux()

	sp, err := NewExampleAppServiceProvider("/tmp/foo", logger)
	if err != nil {
		log.Fatal(err)
	}

	mux.Handle("GET /ping", &ggh.Uitzicht[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandlePing,
		//Middlewares: []ggh.TMiddleware[ExampleAppServiceProvider, struct{}, struct{}, PingResponse]{
		Middlewares: []func(func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, PingGetParams]) (*ggh.GGResponse[PingResponse, ExampleAppErrorData], error)) func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, PingGetParams]) (*ggh.GGResponse[PingResponse, ExampleAppErrorData], error){
			ggh.GetErrorHandlingMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData](HandleErrors),
			ggh.GetDataProcessingMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData](nil),
			ggh.RequestLoggingMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData],
			ggh.RequestIDMiddleware[ExampleAppServiceProvider, struct{}, PingGetParams, PingResponse, ExampleAppErrorData],
		},
		Logger: logger,
	})

	mux.Handle("POST /set_value", &ggh.Uitzicht[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandleSetValue,
		//Middlewares: []ggh.TMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse]{
		Middlewares: []func(func(*ggh.GGRequest[ExampleAppServiceProvider, SetValueRequest, struct{}]) (*ggh.GGResponse[SetValueResponse, ExampleAppErrorData], error)) func(*ggh.GGRequest[ExampleAppServiceProvider, SetValueRequest, struct{}]) (*ggh.GGResponse[SetValueResponse, ExampleAppErrorData], error){
			ggh.GetErrorHandlingMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData](HandleErrors),
			ggh.GetDataProcessingMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData](nil),
			ggh.RequestLoggingMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData],
			ggh.RequestIDMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData],
		},
		Logger: logger,
	})

	mux.Handle("POST /get_value/{key}", &ggh.Uitzicht[ExampleAppServiceProvider, struct{}, struct{}, GetValueResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandleGetValue,
		Middlewares: []func(func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, struct{}]) (*ggh.GGResponse[GetValueResponse, ExampleAppErrorData], error)) func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, struct{}]) (*ggh.GGResponse[GetValueResponse, ExampleAppErrorData], error){
			ggh.GetErrorHandlingMiddleware[ExampleAppServiceProvider, struct{}, struct{}, GetValueResponse, ExampleAppErrorData](HandleErrors),
			ggh.GetDataProcessingMiddleware[ExampleAppServiceProvider, struct{}, struct{}, GetValueResponse, ExampleAppErrorData](nil),
			ggh.RequestLoggingMiddleware[ExampleAppServiceProvider, struct{}, struct{}, GetValueResponse, ExampleAppErrorData],
			ggh.RequestIDMiddleware[ExampleAppServiceProvider, struct{}, struct{}, GetValueResponse, ExampleAppErrorData],
		},
		Logger: logger,
	})

	if err := http.ListenAndServe(":7777", mux); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
