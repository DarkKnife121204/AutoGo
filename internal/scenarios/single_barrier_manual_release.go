package scenarios

import (
	"errors"

	"AutoGo/internal/devices"
	"AutoGo/internal/plcclient"
)

const TypeSingleBarrierManualRelease = "single_barrier_manual_release"

type SingleBarrierManualRelease struct {
	barrier *devices.Barrier
}

func NewSingleBarrierManualRelease(
	barrier *devices.Barrier,
) (*SingleBarrierManualRelease, error) {
	if barrier == nil {
		return nil, errors.New(
			"для сценария single_barrier_manual_release не указан шлагбаум",
		)
	}

	return &SingleBarrierManualRelease{
		barrier: barrier,
	}, nil
}

func (s *SingleBarrierManualRelease) Type() string {
	return TypeSingleBarrierManualRelease
}

func (s *SingleBarrierManualRelease) Start() error {
	return s.barrier.Start()
}

func (s *SingleBarrierManualRelease) StartReverse() error {
	return s.barrier.StartReverse()
}

func (s *SingleBarrierManualRelease) Stop() error {
	return s.barrier.Stop()
}

func (s *SingleBarrierManualRelease) Open() error {
	return s.barrier.Open()
}

func (s *SingleBarrierManualRelease) Close() error {
	return s.barrier.Close()
}

func (s *SingleBarrierManualRelease) Reset() error {
	return s.barrier.Reset()
}

func (s *SingleBarrierManualRelease) Status() (
	plcclient.Status,
	error,
) {
	return s.barrier.Status()
}
