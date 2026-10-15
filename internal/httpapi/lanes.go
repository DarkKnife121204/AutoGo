package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"AutoGo/internal/lanes"
)

func (a *API) laneHandler(
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

func (a *API) laneStatus(
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

	writeJSON(
		w,
		http.StatusOK,
		laneStatusResponse{
			LaneStatus: status,
		},
	)
}

func (a *API) laneCommand(
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

	log.Printf("[lane %s] command: %s", lane.ID, command)

	var err error

	switch command {

	case "start":
		err = lane.Start()

	case "stop":
		err = lane.Stop()

	case "reset":
		err = lane.Reset()

	case "confirm":
		err = lane.Confirm()

	case "reject":
		err = lane.Reject()

	case "next_state":
		err = lane.NextState()

	case "prev_state":
		err = lane.PrevState()

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
		log.Printf("[lane %s] command %s FAILED: %v", lane.ID, command, err)
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

func (a *API) laneContext(
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
	photo, photoCapturedAt, photoCaptureInProgress :=
		lane.Photo()
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

	response := laneContextResponse{
		CurrentState: string(status.State),

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
				PlateType: "",
			},
			CachedData: cachedVehicleDataResponse{
				CachedIdentification: nil,
				CachedValidation:     nil,
			},
		},
	}

	if status.Vehicle != nil {
		response.CurrentDirection = status.Vehicle.Direction
		response.VehicleContext.CurrentVehicle.Direction =
			status.Vehicle.Direction

		if status.Vehicle.Plate != "" {
			plate := status.Vehicle.Plate

			response.VehicleContext.CurrentVehicle.PlateNumber =
				&plate
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

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

func (a *API) writeLanePhotoLast(
	w http.ResponseWriter,
	lane *lanes.Lane,
) {
	photo, _, _ := lane.Photo()

	if len(photo) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"errorType": "PhotoNotFound",
			"message":   "Фото недоступно",
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
