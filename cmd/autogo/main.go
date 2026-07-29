package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"AutoGo/internal/plcclient"
)

const (
	defaultHTTPAddress = ":5001"
	defaultPLCAddress  = "127.0.0.1:5020"
	defaultUnitID      = 1
	defaultPLCTimeout  = 3 * time.Second
)

type application struct {
	plc *plcclient.Client
}

type plcCommandRequest struct {
	Command string `json:"command"`
}

type plcCommandResponse struct {
	Status  string `json:"status"`
	Command string `json:"command"`
}

type plcStatusResponse struct {
	Status string `json:"status"`

	Mode     string `json:"mode"`
	ModeCode int32  `json:"mode_code"`

	State     string `json:"state"`
	StateCode int32  `json:"state_code"`

	ActualState     string `json:"actual_state"`
	ActualStateCode int32  `json:"actual_state_code"`

	LastCommand     string `json:"last_command"`
	LastCommandCode int32  `json:"last_command_code"`

	Alarm   uint16 `json:"alarm"`
	Warning uint16 `json:"warning"`
	Locked  bool   `json:"locked"`

	OperationTime float32 `json:"operation_time"`
	WaitingTime   float32 `json:"waiting_time"`
	Attempts      uint16  `json:"attempts"`
	AttemptPeriod float32 `json:"attempt_period"`
}

func main() {
	httpAddress := getEnv(
		"AUTOGO_HTTP_ADDRESS",
		defaultHTTPAddress,
	)

	plcAddress := getEnv(
		"MODBUS_SERVER_ADDRESS",
		defaultPLCAddress,
	)

	unitID := getUint8(
		"MODBUS_UNIT_ID",
		defaultUnitID,
	)

	plcTimeout := getDuration(
		"MODBUS_TIMEOUT",
		defaultPLCTimeout,
	)

	client, err := plcclient.New(
		plcclient.Config{
			Address: plcAddress,
			UnitID:  unitID,
			Timeout: plcTimeout,
		},
	)
	if err != nil {
		log.Fatalf(
			"не удалось подключиться к PLC: %v",
			err,
		)
	}

	defer func() {
		if err := client.Close(); err != nil {
			log.Printf(
				"ошибка закрытия PLC-соединения: %v",
				err,
			)
		}
	}()

	app := &application{
		plc: client,
	}

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/health",
		app.health,
	)

	mux.HandleFunc(
		"/status",
		app.plcStatus,
	)

	mux.HandleFunc(
		"/command",
		app.plcCommand,
	)

	server := &http.Server{
		Addr:              httpAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf(
			"AutoGo HTTP Server запущен: %s",
			httpAddress,
		)

		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)

	signal.Notify(
		shutdownSignal,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf(
				"ошибка HTTP-сервера: %v",
				err,
			)
		}

	case sig := <-shutdownSignal:
		log.Printf(
			"получен сигнал завершения: %s",
			sig,
		)

		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf(
				"ошибка graceful shutdown: %v",
				err,
			)

			_ = server.Close()
		}
	}
}

func (a *application) health(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		writeJSON(
			w,
			http.StatusMethodNotAllowed,
			map[string]string{
				"status": "error",
				"error":  "method not allowed",
			},
		)

		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"status":  "ok",
			"service": "autogo",
		},
	)
}

func (a *application) plcCommand(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		writeJSON(
			w,
			http.StatusMethodNotAllowed,
			map[string]string{
				"status": "error",
				"error":  "method not allowed",
			},
		)

		return
	}

	var request plcCommandRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "invalid JSON body",
			},
		)

		return
	}

	command := strings.ToLower(
		strings.TrimSpace(request.Command),
	)

	var err error

	switch command {
	case "start":
		err = a.plc.Start()

	case "reverse":
		err = a.plc.StartReverse()

	case "stop":
		err = a.plc.Stop()

	case "open":
		err = a.plc.Open()

	case "close":
		err = a.plc.CloseBarrier()

	case "reset":
		err = a.plc.Reset()

	default:
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "unknown PLC command",
			},
		)

		return
	}

	if err != nil {
		writeJSON(
			w,
			http.StatusServiceUnavailable,
			map[string]string{
				"status": "error",
				"error":  err.Error(),
			},
		)

		return
	}

	writeJSON(
		w,
		http.StatusAccepted,
		plcCommandResponse{
			Status:  "accepted",
			Command: command,
		},
	)
}

func (a *application) plcStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		writeJSON(
			w,
			http.StatusMethodNotAllowed,
			map[string]string{
				"status": "error",
				"error":  "method not allowed",
			},
		)

		return
	}

	status, err := a.plc.Status()
	if err != nil {
		writeJSON(
			w,
			http.StatusServiceUnavailable,
			map[string]string{
				"status": "error",
				"error":  err.Error(),
			},
		)

		return
	}

	response := plcStatusResponse{
		Status: "ok",

		Mode:     status.Mode.String(),
		ModeCode: int32(status.Mode),

		State:     status.State.String(),
		StateCode: int32(status.State),

		ActualState:     status.ActualState.String(),
		ActualStateCode: int32(status.ActualState),

		LastCommand:     status.LastCommand.String(),
		LastCommandCode: int32(status.LastCommand),

		Alarm:   uint16(status.Alarm),
		Warning: status.Warning,
		Locked:  status.Locked,

		OperationTime: status.OperationTime,
		WaitingTime:   status.WaitingTime,
		Attempts:      status.Attempts,
		AttemptPeriod: status.AttemptPeriod,
	}

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

func writeJSON(
	w http.ResponseWriter,
	statusCode int,
	data any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf(
			"ошибка записи JSON-ответа: %v",
			err,
		)
	}
}

func getEnv(
	name string,
	defaultValue string,
) string {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	return value
}

func getDuration(
	name string,
	defaultValue time.Duration,
) time.Duration {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf(
			"неверное значение %s=%q: %v",
			name,
			value,
			err,
		)
	}

	return duration
}

func getUint8(
	name string,
	defaultValue uint8,
) uint8 {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	number, err := strconv.ParseUint(
		value,
		10,
		8,
	)
	if err != nil {
		log.Fatalf(
			"неверное значение %s=%q: %v",
			name,
			value,
			err,
		)
	}

	return uint8(number)
}
