package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"AutoGo/internal/lanes"
	"AutoGo/internal/lanestatus"
)

func (a *API) laneContext(
	w http.ResponseWriter,
	_ *http.Request,
	lane *lanes.Lane,
) {
	context, err := a.makeLaneContext(lane)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"errorType": "InternalError",
			"message":   "Внутренняя ошибка сервера: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, context)
}

func (a *API) makeLaneContext(
	lane *lanes.Lane,
) (laneContextResponse, error) {
	status, err := lane.Status()
	if err != nil {
		return laneContextResponse{}, err
	}

	photo, photoCapturedAt, photoCaptureInProgress := lane.Photo()

	cachedPlate, plateCachedAt, cachedKey, keyCachedAt :=
		lane.CachedIdentification()

	response := laneContextResponse{
		CurrentState:        string(status.State),
		CurrentDirection:    "Unknown",
		IsBidirectionalMode: &lane.BidirectionalMode,

		LastPhotoInfo: photoInfoResponse{
			HasPhoto: false,
		},

		VideoInfo: videoInfoResponse{
			HasVideoRecording: false,
		},

		OperationsStatus: operationsStatusResponse{
			IsPhotoCaptureInProgress: photoCaptureInProgress,
		},

		VehicleContext: vehicleContextResponse{
			CurrentVehicle: currentVehicleResponse{
				Direction: "Unknown",
				PlateType: "Invalid",
			},
			CachedData: cachedVehicleDataResponse{
				CachedIdentification: nil,
				CachedValidation:     nil,
			},
		},
	}

	if status.Vehicle != nil {
		direction := "Unknown"

		switch status.Vehicle.Direction {
		case "normal":
			direction = "Forward"
		case "reverse":
			direction = "Backward"
		}

		response.CurrentDirection = direction
		response.VehicleContext.CurrentVehicle.Direction = direction

		if status.Vehicle.Plate != "" {
			plate := status.Vehicle.Plate
			response.VehicleContext.CurrentVehicle.PlateNumber = &plate
		}
	}

	if len(photo) > 0 {
		fileSize := int64(len(photo))
		captureTime := photoCapturedAt
		fileName := "Photo captured successfully"

		response.LastPhotoInfo = photoInfoResponse{
			HasPhoto:    true,
			CaptureTime: &captureTime,
			FileSize:    &fileSize,
			FileName:    &fileName,
		}
	}

	if cachedPlate != "" || cachedKey != "" {
		identification := cachedIdentificationResponse{
			Direction: "In",
		}

		if cachedPlate != "" {
			plate := cachedPlate
			captureTime := plateCachedAt

			identification.PlateNumber = &plate
			identification.DetectionTime = &captureTime
		}

		if cachedKey != "" {
			key := cachedKey
			identification.KeyCode = &key

			if identification.DetectionTime == nil {
				detectionTime := keyCachedAt
				identification.DetectionTime = &detectionTime
			}
		}

		response.VehicleContext.CachedData.CachedIdentification = &identification
	}

	return response, nil
}

func (a *API) writeLanePhotoLast(
	w http.ResponseWriter,
	lane *lanes.Lane,
) {
	photo, _, _ := lane.Photo()

	if len(photo) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"errorType": "PhotoNotFound",
			"message":   "Фото не найдено",
		})
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(photo); err != nil {
		log.Printf(
			"[lane %s] ошибка отправки JPEG: %v",
			lane.ID,
			err,
		)
	}
}

func (a *API) lanePhotoLast(
	w http.ResponseWriter,
	_ *http.Request,
	lane *lanes.Lane,
) {
	a.writeLanePhotoLast(w, lane)
}

type laneCommandRequest struct {
	CommandType string `json:"commandType"`
}

func (a *API) laneCommand(
	w http.ResponseWriter,
	r *http.Request,
	lane *lanes.Lane,
) {
	var request laneCommandRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidCommand",
			"message":   "Неверный формат команды",
		})
		return
	}

	switch request.CommandType {
	case "NextState", "PrevState", "Reset", "ResetAlarm",
		"Stop", "Start", "Back":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidCommand",
			"message":   "Неизвестная команда",
		})
		return
	}

	status, err := lane.Status()

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"errorType": "InternalError",
			"message":   "Не удалось получить состояние линии: " + err.Error(),
		})
		return
	}

	if !isCommandAllowed(status.State, request.CommandType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidState",
			"message":   "Invalid request - command cannot be executed in current state",
		})
		return
	}

	switch request.CommandType {
	case "NextState":
		err = lane.NextState()

	case "PrevState":
		err = lane.PrevState()

	case "Reset", "ResetAlarm":
		err = lane.Reset()

	case "Start":
		err = lane.Start()

	case "Stop":
		err = lane.Stop()

	case "Back":
		err = lane.Reject()

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidCommand",
			"message":   "Неизвестная или пока не реализованная команда: " + request.CommandType,
		})
		return
	}

	if err != nil {
		log.Printf(
			"[lane %s] command %s failed: %v",
			lane.ID,
			request.CommandType,
			err,
		)

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidState",
			"message":   err.Error(),
		})
		return
	}

	context, err := a.makeLaneContext(lane)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"errorType": "InternalError",
			"message":   "Внутренняя ошибка сервера: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "Команда " + request.CommandType + " выполнена успешно",
		"updatedContext": context,
	})
}

func isCommandAllowed(state lanestatus.State, command string) bool {
	switch state {
	case lanestatus.StateNotReady:
		return command == "Reset" || command == "ResetAlarm"

	case lanestatus.StateIdentEntrance:
		return command == "Reset" || command == "Stop"

	case lanestatus.StateWaitingTransfer1,
		lanestatus.StateWaitingTransfer2:
		return command == "NextState" ||
			command == "PrevState" ||
			command == "Reset" ||
			command == "Stop"

	case lanestatus.StateConfirm:
		return command == "NextState" ||
			command == "PrevState" ||
			command == "Reset" ||
			command == "Stop" ||
			command == "Back"

	case lanestatus.StateAlarm:
		return command == "Reset" ||
			command == "ResetAlarm" ||
			command == "PrevState"

	case lanestatus.StateStopping:
		return command == "Start" || command == "Reset"

	case lanestatus.StateRollingBack:
		return command == "Reset"

	case lanestatus.StateWaitingTransfer:
		return command == "Reset" || command == "Stop"

	default:
		return command == "Reset"
	}
}
