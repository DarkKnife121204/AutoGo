package httpapi

import (
	"time"

	"AutoGo/internal/lanestatus"
)

type plcCommandRequest struct {
	Command string `json:"command"`
	Value   string `json:"value"`
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

type laneStatusResponse struct {
	lanestatus.LaneStatus
}

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
