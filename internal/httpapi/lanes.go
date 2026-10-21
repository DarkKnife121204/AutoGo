package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"AutoGo/internal/lanes"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/video"
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

	cachedPlate, plateCachedAt, cachedKey, keyCachedAt, cachedDirection :=
		lane.CachedIdentification()

	plateNumber, direction, plateType, validationTime, validationInProgress :=
		lane.ContextVehicle()

	validation, validatedPlate, validatedAt, hasCachedValidation :=
		lane.CachedValidation()

	response := laneContextResponse{
		CurrentState:        string(status.State),
		CurrentDirection:    "Unknown",
		IsBidirectionalMode: &lane.BidirectionalMode,

		LastPhotoInfo: photoInfoResponse{
			HasPhoto: false,
		},

		VideoInfo: videoInfoResponse{},

		OperationsStatus: operationsStatusResponse{
			IsPhotoCaptureInProgress: photoCaptureInProgress,
			IsValidationInProgress:   validationInProgress,
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

	recording, videoStart, videoStop := lane.VideoStatus()
	response.VideoInfo.HasVideoRecording = recording
	response.OperationsStatus.IsVideoRecordingInProgress = recording
	if !videoStart.IsZero() {
		response.VideoInfo.StartTime = &videoStart
	}
	if !videoStop.IsZero() {
		response.VideoInfo.StopTime = &videoStop
	}
	if !videoStart.IsZero() && !videoStop.IsZero() {
		seconds := int(videoStop.Sub(videoStart).Seconds())
		duration := fmt.Sprintf("%02d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60)
		response.VideoInfo.RecordingDuration = &duration
	}
	if recording {
		url := "/api/v1/autogate/video/current"
		response.VideoInfo.VideoURL = &url
	}

	if direction != "" {
		apiDirection := "Unknown"

		switch direction {
		case "normal":
			apiDirection = "Forward"
		case "reverse":
			apiDirection = "Backward"
		}

		response.CurrentDirection = apiDirection
		response.VehicleContext.CurrentVehicle.Direction = apiDirection
	}

	if plateNumber != "" {
		plate := plateNumber
		response.VehicleContext.CurrentVehicle.PlateNumber = &plate
	}

	if !validationTime.IsZero() {
		response.VehicleContext.CurrentVehicle.ValidationTime = &validationTime
	}

	if plateType == "Regular" || plateType == "VIP" {
		response.VehicleContext.CurrentVehicle.PlateType = string(plateType)
	}

	if hasCachedValidation {
		valid := validation.Allowed &&
			validation.PlateType != "" &&
			validation.PlateType != "Invalid"

		plateType := string(validation.PlateType)
		if plateType == "" {
			plateType = "Invalid"
		}

		info := validationInfoResponse{
			IsValid:        &valid,
			PlateType:      plateType,
			ValidationTime: &validatedAt,
		}

		if validatedPlate != "" {
			info.PlateNumber = &validatedPlate
		}

		response.VehicleContext.CachedData.CachedValidation = &info
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

		if cachedDirection == "reverse" {
			identification.Direction = "Out"
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
	CommandType json.RawMessage `json:"commandType"`
}

func (a *API) laneCommand(
	w http.ResponseWriter,
	r *http.Request,
	lane *lanes.Lane,
) {
	var request laneCommandRequest

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidCommand",
			"message":   "Неверный формат команды",
		})
		return
	}

	var commandType string

	if err := json.Unmarshal(request.CommandType, &commandType); err != nil {
		var numeric int

		if json.Unmarshal(request.CommandType, &numeric) == nil {
			commands := []string{
				"None", "ForceIdent", "NextState", "Reset", "ResetAlarm",
				"Stop", "Start", "PrevState", "Back",
			}

			if numeric >= 0 && numeric < len(commands) {
				commandType = commands[numeric]
			}
		}
	}

	switch strings.ToLower(commandType) {
	case "forceident":
		commandType = "ForceIdent"
	case "nextstate":
		commandType = "NextState"
	case "prevstate":
		commandType = "PrevState"
	case "reset":
		commandType = "Reset"
	case "resetalarm":
		commandType = "ResetAlarm"
	case "stop":
		commandType = "Stop"
	case "start":
		commandType = "Start"
	case "back":
		commandType = "Back"
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidCommand",
			"message":   "Invalid request - invalid command type",
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

	if !isCommandAllowed(status.State, commandType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"errorType": "InvalidState",
			"message":   "Invalid request - command cannot be executed in current state",
		})
		return
	}

	switch commandType {
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
			"message":   "Invalid request - invalid command type",
		})
		return
	}

	if err != nil {
		log.Printf(
			"[lane %s] command %s failed: %v",
			lane.ID,
			commandType,
			err,
		)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"errorType": "InternalError",
			"message":   "Внутренняя ошибка сервера: " + err.Error(),
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
		"message":        "Команда " + commandType + " выполнена успешно",
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

func (a *API) laneVideoCurrent(w http.ResponseWriter, r *http.Request, lane *lanes.Lane) {
	file, err := lane.CurrentVideoFile()
	if errors.Is(err, video.ErrDownloadUnavailable) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"errorType": "VideoDownloadNotSupported",
			"message":   "Оригинальный Axxon API управляет записью, но не выгружает MP4",
		})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"errorType": "VideoNotFound", "message": "Текущей видеозаписи нет",
		})
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, file)
}
