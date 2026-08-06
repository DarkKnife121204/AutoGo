package scenarios

import "AutoGo/internal/plcclient"

type Scenario interface {
	Type() string

	Start() error
	StartReverse() error
	Stop() error
	Open() error
	Close() error
	Reset() error

	Status() (plcclient.Status, error)
}
