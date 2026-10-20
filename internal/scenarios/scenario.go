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

	Advance(validationValid bool, vip bool) (done bool, err error)

	NextState(validationValid bool, vip bool) (done bool, err error)
	PrevState(validationValid bool, vip bool) error

	Confirm() error
	Reject() error

	Start() error
	StartReverse() error
	Stop() error
	Open() error
	Close() error
	Reset() ([]string, error)
	Cancel() error
	RecoverNotReady() ([]string, error)
	Resume() error
	StopInit(barrierID string) error

	Snapshot() (lanestatus.Snapshot, error)
}
