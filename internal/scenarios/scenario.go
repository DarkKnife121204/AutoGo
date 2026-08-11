package scenarios

import "AutoGo/internal/lanestatus"

type TriggerSource string

const (
	TriggerSourceAPI    TriggerSource = "api"
	TriggerSourceCamera TriggerSource = "camera"
	TriggerSourceCode   TriggerSource = "code"
)

type Scenario interface {
	Type() string

	ReleaseMode() string
	Trigger(source TriggerSource) error

	Start() error
	StartReverse() error
	Stop() error
	Open() error
	Close() error
	Reset() error

	Status() (lanestatus.LaneStatus, error)
}
