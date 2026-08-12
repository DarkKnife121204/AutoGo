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

	Begin(direction string) error

	Advance() (done bool, err error)

	Confirm() error
	Reject() error

	Start() error
	StartReverse() error
	Stop() error
	Open() error
	Close() error
	Reset() error

	Snapshot() (lanestatus.Snapshot, error)
}
