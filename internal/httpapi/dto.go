package httpapi

import (
	"time"
)

type laneSummaryResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Scenario string `json:"scenario"`
}

type checkpointResponse struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Lanes []laneSummaryResponse `json:"lanes"`
}

type checkpointListResponse struct {
	Status      string               `json:"status"`
	Checkpoints []checkpointResponse `json:"checkpoints"`
}

type laneContextResponse struct {
	CurrentState        string                   `json:"currentState"`
	CurrentDirection    string                   `json:"currentDirection"`
	IsBidirectionalMode *bool                    `json:"isBidirectionalMode"`
	LastPhotoInfo       photoInfoResponse        `json:"lastPhotoInfo"`
	VideoInfo           videoInfoResponse        `json:"videoInfo"`
	OperationsStatus    operationsStatusResponse `json:"operationsStatus"`
	VehicleContext      vehicleContextResponse   `json:"vehicleContext"`
}

type photoInfoResponse struct {
	HasPhoto    bool       `json:"hasPhoto"`
	CaptureTime *time.Time `json:"captureTime"`
	FileSize    *int64     `json:"fileSize"`
	FileName    *string    `json:"fileName"`
}

type videoInfoResponse struct {
	HasVideoRecording bool       `json:"hasVideoRecording"`
	StartTime         *time.Time `json:"startTime"`
	StopTime          *time.Time `json:"stopTime"`
	RecordingDuration *string    `json:"recordingDuration"`
	VideoURL          *string    `json:"videoUrl"`
}

type operationsStatusResponse struct {
	IsPhotoCaptureInProgress   bool `json:"isPhotoCaptureInProgress"`
	IsValidationInProgress     bool `json:"isValidationInProgress"`
	IsVideoRecordingInProgress bool `json:"isVideoRecordingInProgress"`
}

type vehicleContextResponse struct {
	CurrentVehicle currentVehicleResponse    `json:"currentVehicle"`
	CachedData     cachedVehicleDataResponse `json:"cachedData"`
}

type currentVehicleResponse struct {
	PlateNumber    *string    `json:"plateNumber"`
	Direction      string     `json:"direction"`
	ValidationTime *time.Time `json:"validationTime"`
	PlateType      string     `json:"plateType"`
}

type cachedVehicleDataResponse struct {
	CachedIdentification *cachedIdentificationResponse `json:"cachedIdentification"`
	CachedValidation     *validationInfoResponse       `json:"cachedValidation"`
}

type cachedIdentificationResponse struct {
	PlateNumber   *string    `json:"plateNumber"`
	KeyCode       *string    `json:"keyCode"`
	Direction     string     `json:"direction"`
	DetectionTime *time.Time `json:"detectionTime"`
}

type validationInfoResponse struct {
	PlateNumber    *string    `json:"plateNumber"`
	IsValid        *bool      `json:"isValid"`
	PlateType      string     `json:"plateType"`
	ValidationTime *time.Time `json:"validationTime"`
}
