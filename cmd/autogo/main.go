package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"AutoGo/internal/config"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plcclient"
)

const (
	defaultHTTPAddress = ":5001"
)

type application struct {
	controllers map[string]*plcclient.Client
	barriers    map[string]*devices.Barrier
	lanes       map[string]*lanes.Lane
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
	configPath := getEnv(
		"AUTOGO_CONFIG_PATH",
		"config/site.yaml",
	)

	siteConfig, err := config.Load(configPath)
	if err != nil {
		log.Fatalf(
			"не удалось загрузить конфигурацию AutoGo: %v",
			err,
		)
	}

	log.Printf(
		"конфигурация загружена: site=%s controllers=%d devices=%d checkpoints=%d",
		siteConfig.Site.ID,
		len(siteConfig.Controllers),
		len(siteConfig.Devices),
		len(siteConfig.Checkpoints),
	)

	httpAddress := getEnv(
		"AUTOGO_HTTP_ADDRESS",
		defaultHTTPAddress,
	)

	controllers, err := buildPLCClients(
		siteConfig.Controllers,
	)
	if err != nil {
		log.Fatalf(
			"не удалось создать PLC-клиенты: %v",
			err,
		)
	}

	defer closePLCClients(controllers)

	log.Printf(
		"PLC-клиенты созданы: count=%d",
		len(controllers),
	)

	barriers, err := buildBarriers(
		siteConfig.Devices,
		controllers,
	)
	if err != nil {
		log.Fatalf(
			"не удалось создать устройства: %v",
			err,
		)
	}

	log.Printf(
		"устройства созданы: barriers=%d",
		len(barriers),
	)

	siteLanes, err := buildLanes(
		siteConfig.Checkpoints,
		barriers,
	)
	if err != nil {
		log.Fatalf(
			"не удалось создать линии: %v",
			err,
		)
	}

	log.Printf(
		"линии созданы: count=%d",
		len(siteLanes),
	)

	app := &application{
		controllers: controllers,
		barriers:    barriers,
		lanes:       siteLanes,
	}

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/health",
		app.health,
	)

	mux.HandleFunc(
		"/barriers/",
		app.barrierHandler,
	)

	mux.HandleFunc(
		"/lanes/",
		app.laneHandler,
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

func (a *application) laneStatus(
	w http.ResponseWriter,
	r *http.Request,
	lane *lanes.Lane,
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

	status, err := lane.Status()
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

func (a *application) laneCommand(
	w http.ResponseWriter,
	r *http.Request,
	lane *lanes.Lane,
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
		err = lane.Start()

	case "reverse":
		err = lane.StartReverse()

	case "stop":
		err = lane.Stop()

	case "open":
		err = lane.Open()

	case "close":
		err = lane.Close()

	case "reset":
		err = lane.Reset()

	default:
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "unknown lane command",
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

func (a *application) laneHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	path := strings.TrimPrefix(
		r.URL.Path,
		"/lanes/",
	)

	parts := strings.Split(
		strings.Trim(path, "/"),
		"/",
	)

	if len(parts) != 2 {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "route not found",
			},
		)

		return
	}

	laneID := parts[0]
	action := parts[1]

	lane, exists := a.lanes[laneID]
	if !exists {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "lane not found",
			},
		)

		return
	}

	switch action {
	case "status":
		a.laneStatus(w, r, lane)

	case "command":
		a.laneCommand(w, r, lane)

	default:
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "route not found",
			},
		)
	}
}

func (a *application) barrierHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	path := strings.TrimPrefix(
		r.URL.Path,
		"/barriers/",
	)

	parts := strings.Split(
		strings.Trim(path, "/"),
		"/",
	)

	if len(parts) != 2 {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "route not found",
			},
		)

		return
	}

	barrierID := parts[0]
	action := parts[1]

	barrier, exists := a.barriers[barrierID]
	if !exists {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "barrier not found",
			},
		)

		return
	}

	switch action {
	case "status":
		a.barrierStatus(w, r, barrier)

	case "command":
		a.barrierCommand(w, r, barrier)

	default:
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "route not found",
			},
		)
	}
}

func buildPLCClients(
	controllers []config.ControllerConfig,
) (map[string]*plcclient.Client, error) {
	clients := make(
		map[string]*plcclient.Client,
		len(controllers),
	)

	for _, controller := range controllers {
		if !controller.Enabled {
			continue
		}

		client, err := plcclient.New(
			plcclient.Config{
				Address: controller.Address,
				UnitID:  controller.UnitID,
				Timeout: controller.Timeout,
			},
		)
		if err != nil {
			closePLCClients(clients)

			return nil, fmt.Errorf(
				"создание PLC-клиента %q: %w",
				controller.ID,
				err,
			)
		}

		clients[controller.ID] = client
	}

	if len(clients) == 0 {
		return nil, errors.New(
			"в конфигурации нет включённых PLC-контроллеров",
		)
	}

	return clients, nil
}
func closePLCClients(
	clients map[string]*plcclient.Client,
) {
	for controllerID, client := range clients {
		if err := client.Close(); err != nil {
			log.Printf(
				"ошибка закрытия PLC-клиента %s: %v",
				controllerID,
				err,
			)
		}
	}
}

func buildBarriers(
	deviceConfigs []config.DeviceConfig,
	controllers map[string]*plcclient.Client,
) (map[string]*devices.Barrier, error) {
	barriers := make(map[string]*devices.Barrier)

	for _, deviceConfig := range deviceConfigs {
		if !deviceConfig.Enabled {
			continue
		}

		if deviceConfig.Type != config.DeviceTypeBarrier {
			continue
		}

		controller, exists := controllers[deviceConfig.Controller]
		if !exists {
			return nil, fmt.Errorf(
				"для шлагбаума %q не найден контроллер %q",
				deviceConfig.ID,
				deviceConfig.Controller,
			)
		}

		barrier, err := devices.NewBarrier(
			deviceConfig.ID,
			deviceConfig.Name,
			deviceConfig.Controller,
			controller,
		)
		if err != nil {
			return nil, err
		}

		barriers[deviceConfig.ID] = barrier
	}

	if len(barriers) == 0 {
		return nil, errors.New(
			"в конфигурации нет включённых шлагбаумов",
		)
	}

	return barriers, nil
}

func buildLanes(
	checkpoints []config.CheckpointConfig,
	barriers map[string]*devices.Barrier,
) (map[string]*lanes.Lane, error) {
	result := make(map[string]*lanes.Lane)

	for _, checkpoint := range checkpoints {
		for _, laneConfig := range checkpoint.Lanes {
			if !laneConfig.Enabled {
				continue
			}

			switch laneConfig.Scenario.Type {
			case config.ScenarioTypeSingleBarrier:
				barrierID := laneConfig.Scenario.Settings["barrier"]

				barrier, exists := barriers[barrierID]
				if !exists {
					return nil, fmt.Errorf(
						"для линии %q не найден шлагбаум %q",
						laneConfig.ID,
						barrierID,
					)
				}

				lane, err := lanes.NewSingleBarrier(
					laneConfig.ID,
					laneConfig.Name,
					checkpoint.ID,
					barrier,
				)
				if err != nil {
					return nil, err
				}

				result[lane.ID] = lane

			default:
				return nil, fmt.Errorf(
					"линия %q использует неизвестный сценарий %q",
					laneConfig.ID,
					laneConfig.Scenario.Type,
				)
			}
		}
	}

	if len(result) == 0 {
		return nil, errors.New(
			"в конфигурации нет включённых линий",
		)
	}

	return result, nil
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

func (a *application) barrierCommand(
	w http.ResponseWriter,
	r *http.Request,
	barrier *devices.Barrier,
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
		err = barrier.Start()

	case "reverse":
		err = barrier.StartReverse()

	case "stop":
		err = barrier.Stop()

	case "open":
		err = barrier.Open()

	case "close":
		err = barrier.Close()

	case "reset":
		err = barrier.Reset()

	default:
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "unknown barrier command",
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

func (a *application) barrierStatus(
	w http.ResponseWriter,
	r *http.Request,
	barrier *devices.Barrier,
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

	status, err := barrier.Status()
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
