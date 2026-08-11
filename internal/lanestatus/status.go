package lanestatus

type Phase string

const (
	PhaseIdle                Phase = "idle"
	PhaseOpening             Phase = "opening"
	PhaseOpened              Phase = "opened"
	PhaseClosing             Phase = "closing"
	PhaseError               Phase = "error"
	PhaseWaitingConfirmation Phase = "waiting_confirmation"
)

type DeviceStatus struct {
	DeviceID string `json:"device_id"`
	Type     string `json:"type"`
	State    string `json:"state"`
}

type VehicleInfo struct {
	ID        string `json:"id"`
	Value     string `json:"value,omitempty"`
	Source    string `json:"source"`
	Stage     string `json:"stage"`
	StartedAt int64  `json:"started_at"`
}

type LaneStatus struct {
	LaneID      string                  `json:"lane_id"`
	Mode        string                  `json:"mode"`
	Scenario    string                  `json:"scenario"`
	ReleaseMode string                  `json:"release_mode"`
	Direction   string                  `json:"direction"`
	Phase       Phase                   `json:"phase"`
	Ready       bool                    `json:"ready"`
	Busy        bool                    `json:"busy"`
	Allowed     *bool                   `json:"allowed,omitempty"`
	Alarm       bool                    `json:"alarm"`
	Vehicle     *VehicleInfo            `json:"vehicle,omitempty"`
	Devices     map[string]DeviceStatus `json:"devices"`
}
