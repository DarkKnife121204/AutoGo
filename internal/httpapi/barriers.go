package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plc"
	"AutoGo/internal/plcclient"
)

func (a *API) barrierForLane(lane *lanes.Lane, name string) (*devices.Barrier, bool) {
	barrier, ok := a.gateBarriers[lane.ID][name]
	return barrier, ok && barrier != nil
}

func writeGateResponse(w http.ResponseWriter, code int, success bool, message string, data any) {
	writeJSON(w, code, map[string]any{
		"success": success,
		"message": message,
		"data":    data,
	})
}

func (a *API) gateNames(lane *lanes.Lane) []string {
	names := make([]string, 0, len(a.gateBarriers[lane.ID]))

	for name := range a.gateBarriers[lane.ID] {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func (a *API) gateServiceEnabled(lane *lanes.Lane) bool {
	for _, barrier := range a.gateBarriers[lane.ID] {
		if barrier.IsLocked() {
			return false
		}
	}

	return true
}

func gateAlarmName(value plc.Alarm) (string, error) {

	switch value {
	case plc.AlarmNone:
		return "None", nil
	case plc.AlarmBarrierAlarmOpen:
		return "BarrierAlarmOpen", nil
	case plc.AlarmBarrierAlarmClose:
		return "BarrierAlarmClose", nil
	case plc.AlarmBarrierIllegalOpen:
		return "BarrierIllegalOpen", nil
	case plc.AlarmUnallowedStart:
		return "UnallowedStart", nil
	case plc.AlarmNoEvent:
		return "NoEvent", nil
	default:
		return "", fmt.Errorf("неизвестная авария PLC: %d", value)
	}
}

func (a *API) barrierStatus(lane *lanes.Lane, name string) (plcclient.Status, bool, error) {
	barrier, ok := a.barrierForLane(lane, name)

	if !ok {
		return plcclient.Status{}, false, nil
	}

	status, err := barrier.Status()

	return status, true, err
}

func (a *API) barrierState(w http.ResponseWriter, r *http.Request, lane *lanes.Lane) {
	name := r.PathValue("barrier")

	status, exists, err := a.barrierStatus(lane, name)

	if !exists {
		writeGateResponse(w, http.StatusNotFound, false, "Gate with id '"+name+"' not found", nil)
		return
	}

	if err != nil {
		writeGateResponse(w, http.StatusInternalServerError, false, "Error retrieving state: "+err.Error(), nil)
		return
	}

	if status.State.String() == "Unknown" {
		writeGateResponse(w, http.StatusInternalServerError, false, "Неизвестное состояние PLC", nil)
		return
	}

	writeGateResponse(w, http.StatusOK, true, "State retrieved successfully", status.State.String())
}

func (a *API) barrierAlarm(w http.ResponseWriter, r *http.Request, lane *lanes.Lane) {
	name := r.PathValue("barrier")

	status, exists, err := a.barrierStatus(lane, name)

	if !exists {
		writeGateResponse(w, http.StatusNotFound, false, "Gate with id '"+name+"' not found", nil)
		return
	}

	if err != nil {
		writeGateResponse(w, http.StatusInternalServerError, false, "Error retrieving alarm state: "+err.Error(), nil)
		return
	}

	alarm, err := gateAlarmName(status.Alarm)

	if err != nil {
		writeGateResponse(w, http.StatusInternalServerError, false, err.Error(), nil)
		return
	}

	writeGateResponse(w, http.StatusOK, true, "Alarm state retrieved successfully", alarm)
}

func (a *API) gateInfo(lane *lanes.Lane, name string) (map[string]any, bool, error) {
	status, exists, err := a.barrierStatus(lane, name)

	if !exists || err != nil {
		return nil, exists, err
	}

	alarm, err := gateAlarmName(status.Alarm)

	if err != nil {
		return nil, true, err
	}

	if status.State.String() == "Unknown" {
		return nil, true, fmt.Errorf("неизвестное состояние PLC: %d", status.State)
	}

	return map[string]any{
		"id":          name,
		"state":       status.State.String(),
		"alarm":       alarm,
		"warning":     "None",
		"timeWaiting": int(status.WaitingTime),
	}, true, nil
}

func (a *API) barrierInfo(w http.ResponseWriter, r *http.Request, lane *lanes.Lane) {
	name := r.PathValue("barrier")

	data, exists, err := a.gateInfo(lane, name)

	if !exists {
		writeGateResponse(w, http.StatusNotFound, false, "Gate with id '"+name+"' not found", nil)
		return
	}

	if err != nil {
		writeGateResponse(w, http.StatusInternalServerError, false, "Error retrieving gate: "+err.Error(), nil)
		return
	}

	writeGateResponse(w, http.StatusOK, true, "Gate retrieved successfully", data)
}

func (a *API) barrierList(w http.ResponseWriter, _ *http.Request, lane *lanes.Lane) {
	data := make([]map[string]any, 0, len(a.gateBarriers[lane.ID]))

	for _, name := range a.gateNames(lane) {
		info, _, err := a.gateInfo(lane, name)
		if err != nil {
			writeGateResponse(w, http.StatusInternalServerError, false, "Error retrieving gates: "+err.Error(), nil)
			return
		}
		data = append(data, info)
	}

	writeGateResponse(w, http.StatusOK, true, "Gates retrieved successfully", data)
}

func (a *API) barrierCount(w http.ResponseWriter, _ *http.Request, lane *lanes.Lane) {
	writeGateResponse(w, http.StatusOK, true, "Gate count retrieved successfully", len(a.gateBarriers[lane.ID]))
}

func (a *API) barrierServiceStatus(w http.ResponseWriter, _ *http.Request, lane *lanes.Lane) {
	writeGateResponse(w, http.StatusOK, true, "Service status retrieved successfully", a.gateServiceEnabled(lane))
}

func (a *API) barrierServiceCommand(w http.ResponseWriter, r *http.Request, lane *lanes.Lane) {
	command := strings.ToLower(r.PathValue("command"))

	if command != "enable" && command != "disable" {
		http.NotFound(w, r)
		return
	}

	locked := command == "disable"

	for _, barrier := range a.gateBarriers[lane.ID] {
		barrier.SetLocked(locked)
	}

	if locked {
		writeGateResponse(w, http.StatusOK, true, "Service disabled successfully", "сервис заблокирован САУ")
	} else {
		writeGateResponse(w, http.StatusOK, true, "Service enabled successfully", "сервис разблокирован")
	}
}

func (a *API) barrierCommand(w http.ResponseWriter, r *http.Request, lane *lanes.Lane) {
	name := r.PathValue("barrier")

	barrier, ok := a.barrierForLane(lane, name)

	if !ok {
		writeGateResponse(w, http.StatusNotFound, false, "Gate with id '"+name+"' not found", nil)
		return
	}

	if !a.gateServiceEnabled(lane) {
		writeGateResponse(w, http.StatusLocked, false, "сервис заблокирован САУ", nil)
		return
	}

	if lane.Mode() != lanes.ModeManual {
		writeGateResponse(w, http.StatusLocked, false, "Прямое управление запрещено: линия в automatic", nil)
		return
	}

	command := strings.ToLower(r.PathValue("command"))

	if command == "command" {
		var request struct {
			Command json.RawMessage `json:"command"`
		}
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&request); err != nil || len(request.Command) == 0 {
			writeGateResponse(w, http.StatusBadRequest, false, "Command is required", nil)
			return
		}
		var text string
		if json.Unmarshal(request.Command, &text) == nil {
			command = strings.ToLower(text)
		} else {
			var number int
			if json.Unmarshal(request.Command, &number) != nil {
				writeGateResponse(w, http.StatusBadRequest, false, "Неизвестная команда", nil)
				return
			}
			switch number {
			case 1:
				command = "stop"
			case 2:
				command = "start"
			case 3:
				command = "startreverse"
			case 4:
				command = "open"
			case 32768:
				command = "reset"
			default:
				command = ""
			}
		}
	}
	switch command {
	case "start", "startreverse", "stop", "reset", "resetalarm", "open", "close":
	default:
		writeGateResponse(w, http.StatusBadRequest, false, "Неизвестная команда", nil)
		return
	}

	err := lane.ManualBarrierCommand(barrier, command)

	if errors.Is(err, lanes.ErrManualCommandForbidden) {
		writeGateResponse(w, http.StatusLocked, false, "Прямое управление запрещено: линия в automatic", nil)
		return
	}

	if err != nil {
		writeGateResponse(w, http.StatusInternalServerError, false, "Error executing command: "+err.Error(), nil)
		return
	}

	writeGateResponse(w, http.StatusOK, true, "Command '"+command+"' executed successfully", "Command "+command+" sent to gate "+name)
}
