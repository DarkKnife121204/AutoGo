package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plc"
)

func (a *API) barrierForLane(
	lane *lanes.Lane,
	name string,
) (*devices.Barrier, bool) {
	barrier, ok := a.gateBarriers[lane.ID][name]
	return barrier, ok && barrier != nil
}

func (a *API) barrierState(
	w http.ResponseWriter,
	r *http.Request,
	lane *lanes.Lane,
) {
	barrier, ok := a.barrierForLane(lane, r.PathValue("barrier"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Шлагбаум не найден",
			"data":    nil,
		})
		return
	}

	status, err := barrier.Status()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	var state int

	switch status.State {
	case plc.StateInit:
		state = 0
	case plc.StateClosed:
		state = 1
	case plc.StateOpening:
		state = 2
	case plc.StateOpened:
		state = 3
	case plc.StateClosing:
		state = 4
	case plc.StateAlarm:
		state = 5
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Нет соответствия состояния PLC состоянию UDTGate",
			"data":    nil,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "OK",
		"data":    state,
	})
}

func (a *API) barrierCommand(
	w http.ResponseWriter,
	r *http.Request,
	lane *lanes.Lane,
) {
	barrier, ok := a.barrierForLane(lane, r.PathValue("barrier"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Шлагбаум не найден",
			"data":    nil,
		})
		return
	}

	if lane.Mode() == lanes.ModeAutomatic {
		writeJSON(w, http.StatusLocked, map[string]any{
			"success": false,
			"message": "Прямое управление запрещено: линия в automatic",
			"data":    nil,
		})
		return
	}

	command := strings.ToLower(r.PathValue("command"))

	if command == "command" {
		var request struct {
			Command *int `json:"command"`
		}

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&request); err != nil || request.Command == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"message": "Неверный формат команды",
				"data":    nil,
			})
			return
		}

		switch *request.Command {
		case 1:
			command = "stop"
		case 2:
			command = "start"
		case 3:
			command = "startreverse"
		case 4:
			command = "open"
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"message": "Неизвестная команда",
				"data":    nil,
			})
			return
		}
	}

	var err error

	switch command {
	case "start":
		err = barrier.Start()
	case "startreverse":
		err = barrier.StartReverse()
	case "stop":
		err = barrier.Stop()
	case "reset", "resetalarm":
		err = barrier.Reset()
	case "open":
		err = barrier.Open()
	case "close":
		err = barrier.Close()
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Неизвестная команда",
			"data":    nil,
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Команда выполнена успешно",
		"data":    command,
	})
}
