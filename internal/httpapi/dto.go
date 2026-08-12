package httpapi

import "AutoGo/internal/lanestatus"

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
