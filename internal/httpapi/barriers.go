package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
)

func (a *API) barrierHandler(
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

func (a *API) barrierStatus(
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

func (a *API) barrierCommand(
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

	if lane, ok := a.barrierLane[barrier.ID]; ok &&
		lane.Mode() == lanes.ModeAutomatic {
		log.Printf(
			"[barrier %s] command rejected (interlock): lane in automatic",
			barrier.ID,
		)
		writeJSON(
			w,
			http.StatusConflict,
			map[string]string{
				"status": "error",
				"error":  "прямое управление запрещено: линия в режиме automatic, остановите автоматику линии",
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
		log.Printf(
			"[barrier %s] command %s FAILED: %v",
			barrier.ID, command, err,
		)
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

	log.Printf("[barrier %s] command %s accepted", barrier.ID, command)

	writeJSON(
		w,
		http.StatusAccepted,
		plcCommandResponse{
			Status:  "accepted",
			Command: command,
		},
	)
}
