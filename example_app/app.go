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

func HandleErrors(err error, l *slog.Logger) (statusCode int, errorData ExampleAppErrorData) {
	l.Warn("Handling error", slog.String("error", err.Error()))
	switch err.(type) {
	case RandomError:
		statusCode, errorData = 418, ExampleAppErrorData{Code: "TEAPOT", Message: err.Error(), Details: map[string]string{"reason": "destiny"}}
	case DatabaseError:
		statusCode, errorData = 424, ExampleAppErrorData{Code: "DATABASE", Message: err.Error(), Details: nil}
	default:
		statusCode, errorData = 500, ExampleAppErrorData{Code: "INTERNAL", Message: err.Error()}
	}
	l.Info("Handled error", slog.Int("status_code", statusCode), slog.String("code", errorData.Code))
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
	filePath string
}

func NewTheStorage(filePath string) (*TheStorage, error) {
	db, err := sql.Open("sqlite3", filePath)
	defer db.Close()
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS storage (key string NOT NULL PRIMARY KEY, value string)`)
	if err != nil {
		return nil, err
	}
	return &TheStorage{filePath: filePath}, nil
}

func (ts *TheStorage) getDb() (*sql.DB, error) {
	return sql.Open("sqlite3", ts.filePath)
}

func (ts *TheStorage) Get(key string) (any, error) {
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
	return value, nil
}

func (ts *TheStorage) Set(key string, value any) error {
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
	storage *TheStorage
}

func NewExampleAppServiceProvider(filePath string) (*ExampleAppServiceProvider, error) {
	easp := &ExampleAppServiceProvider{}
	var err error
	easp.storage, err = NewTheStorage(filePath)
	if err != nil {
		return nil, err
	}
	return easp, nil
}

func (sp *ExampleAppServiceProvider) GetStorage() *TheStorage {
	return sp.storage
}

type PingResponse struct {
	Message string `json:"message"`
}

func HandlePing(ggreq *ggh.GGRequest[ExampleAppServiceProvider, struct{}, struct{}]) (ggh.GGResponse[PingResponse], error) {
	ggreq.Logger.Info("Preparing pong...")
	if rand.Intn(2) == 1 {
		return ggh.GGResponse[PingResponse]{nil, nil}, RandomError{}
	}
	return ggh.GGResponse[PingResponse]{
		&PingResponse{
			Message: "Pong",
		},
		nil,
	}, nil
}

type SetValueRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SetValueResponse struct {
	Message string `json:"message"`
}

func HandleSetValue(ggreq *ggh.GGRequest[ExampleAppServiceProvider, SetValueRequest, struct{}]) (ggh.GGResponse[SetValueResponse], error) {
	storage := ggreq.ServiceProvider.GetStorage()
	err := storage.Set(ggreq.RequestData.Key, ggreq.RequestData.Value)
	if err != nil {
		return ggh.GGResponse[SetValueResponse]{nil, nil}, DatabaseError{DBMessage: err.Error()}
	}
	return ggh.GGResponse[SetValueResponse]{
		&SetValueResponse{Message: "ok"},
		nil,
	}, nil
}

func main() {
	mux := http.NewServeMux()

	sp, err := NewExampleAppServiceProvider("/tmp/foo")
	if err != nil {
		log.Fatal(err)
	}

	loggingHandler := slog.NewJSONHandler(os.Stdout, nil)
	logger := slog.New(loggingHandler)

	mux.Handle("GET /ping", &ggh.Uitzicht[ExampleAppServiceProvider, struct{}, struct{}, PingResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandlePing,
		//Middlewares: []ggh.TMiddleware[ExampleAppServiceProvider, struct{}, struct{}, PingResponse]{
		Middlewares: []func(func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, struct{}]) (ggh.GGResponse[PingResponse], error)) func(*ggh.GGRequest[ExampleAppServiceProvider, struct{}, struct{}]) (ggh.GGResponse[PingResponse], error){
			ggh.RequestIDMiddleware[ExampleAppServiceProvider, struct{}, struct{}, PingResponse],
			ggh.RequestLoggingMiddleware[ExampleAppServiceProvider, struct{}, struct{}, PingResponse],
		},
		ErrorHandler: HandleErrors,
		Logger:       logger,
	})

	mux.Handle("POST /set_value", &ggh.Uitzicht[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse, ExampleAppErrorData]{
		ServiceProvider: sp,
		HandlerFunc:     HandleSetValue,
		//Middlewares: []ggh.TMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse]{
		Middlewares: []func(func(*ggh.GGRequest[ExampleAppServiceProvider, SetValueRequest, struct{}]) (ggh.GGResponse[SetValueResponse], error)) func(*ggh.GGRequest[ExampleAppServiceProvider, SetValueRequest, struct{}]) (ggh.GGResponse[SetValueResponse], error){
			ggh.RequestLoggingMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse],
			ggh.RequestIDMiddleware[ExampleAppServiceProvider, SetValueRequest, struct{}, SetValueResponse],
		},
		Logger: logger,
	})

	if err := http.ListenAndServe(":7777", mux); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
