package lanestatus

type Phase string

const (
	PhaseIdle    Phase = "idle"
	PhaseOpening Phase = "opening"
	PhaseOpened  Phase = "opened"
	PhaseClosing Phase = "closing"
	PhaseError   Phase = "error"
)

type State string

const (
	StateAlarm    State = "Alarm"
	StateStopping State = "Stopping"

	StateIdentEntrance   State = "IdentEntrance"
	StateWaitingTransfer State = "WaitingTransfer"

	StateWaitingTransfer1 State = "WaitingTransfer1"
	StateConfirm          State = "Confirm"
	StateWaitingTransfer2 State = "WaitingTransfer2"

	StateRollingBack State = "RollingBack"
)

type DeviceStatus struct {
	Type       string `json:"type"`
	State      string `json:"state,omitempty"`
	Controller string `json:"controller,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

type VehicleInfo struct {
	ID        string `json:"id"`
	Plate     string `json:"plate,omitempty"`
	KeyCode   string `json:"key_code,omitempty"`
	Direction string `json:"direction,omitempty"`
	Source    string `json:"source"`
	Stage     string `json:"stage"`
	StartedAt int64  `json:"started_at"`
}

type Snapshot struct {
	Phase   Phase
	Alarm   bool
	Stage   string
	Devices map[string]DeviceStatus
}

type QueuedInfo struct {
	ID        string `json:"id"`
	Plate     string `json:"plate,omitempty"`
	KeyCode   string `json:"key_code,omitempty"`
	Source    string `json:"source"`
	StartedAt int64  `json:"started_at"`
}

type LaneStatus struct {
	LaneID      string                  `json:"lane_id"`
	Scenario    string                  `json:"scenario"`
	ReleaseMode string                  `json:"release_mode"`
	State       State                   `json:"state"`
	Vehicle     *VehicleInfo            `json:"vehicle,omitempty"`
	Queue       []QueuedInfo            `json:"queue,omitempty"`
	Devices     map[string]DeviceStatus `json:"devices"`
}
