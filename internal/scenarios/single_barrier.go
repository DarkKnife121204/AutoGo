package scenarios

import (
	"errors"
	"fmt"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/plc"
)

const TypeSingleBarrier = "single_barrier"

const (
	defaultReleaseMode = "immediate"
	defaultDirection   = "normal"

	directionReverse = "reverse"
)

type SingleBarrier struct {
	barrier *devices.Barrier

	releaseMode string
	direction   string
	triggers    []string
}

func NewSingleBarrier(
	barrier *devices.Barrier,
	releaseMode string,
	direction string,
	triggers []string,
) (*SingleBarrier, error) {
	if barrier == nil {
		return nil, errors.New(
			"для сценария single_barrier не указан шлагбаум",
		)
	}

	if releaseMode == "" {
		releaseMode = defaultReleaseMode
	}

	if direction == "" {
		direction = defaultDirection
	}

	return &SingleBarrier{
		barrier:     barrier,
		releaseMode: releaseMode,
		direction:   direction,
		triggers:    triggers,
	}, nil
}

func (s *SingleBarrier) Type() string {
	return TypeSingleBarrier
}

func (s *SingleBarrier) Trigger(source TriggerSource) error {
	if s.releaseMode != defaultReleaseMode {
		return fmt.Errorf(
			"release_mode %q пока не реализован",
			s.releaseMode,
		)
	}

	if s.direction == directionReverse {
		return s.barrier.StartReverse()
	}

	return s.barrier.Start()
}

func (s *SingleBarrier) Start() error {
	return s.barrier.Start()
}

func (s *SingleBarrier) StartReverse() error {
	return s.barrier.StartReverse()
}

func (s *SingleBarrier) Stop() error {
	return s.barrier.Stop()
}

func (s *SingleBarrier) Open() error {
	return s.barrier.Open()
}

func (s *SingleBarrier) Close() error {
	return s.barrier.Close()
}

func (s *SingleBarrier) Reset() error {
	return s.barrier.Reset()
}

func (s *SingleBarrier) Status() (lanestatus.LaneStatus, error) {
	plcStatus, err := s.barrier.Status()
	if err != nil {
		return lanestatus.LaneStatus{}, err
	}

	phase := phaseFromState(plcStatus.State)

	return lanestatus.LaneStatus{
		Scenario:    s.Type(),
		ReleaseMode: s.releaseMode,
		Direction:   s.direction,
		Triggers:    s.triggers,
		Phase:       phase,
		Ready:       phase == lanestatus.PhaseIdle,
		Busy:        phase == lanestatus.PhaseOpening || phase == lanestatus.PhaseOpened || phase == lanestatus.PhaseClosing,
		Allowed:     nil,
		Alarm:       plcStatus.HasAlarm(),
		Devices: map[string]lanestatus.DeviceStatus{
			s.barrier.ID: {
				DeviceID: s.barrier.ID,
				Type:     "barrier",
				State:    plcStatus.State.String(),
			},
		},
	}, nil
}

func phaseFromState(state plc.State) lanestatus.Phase {
	switch state {
	case plc.StateOpening:
		return lanestatus.PhaseOpening
	case plc.StateOpened:
		return lanestatus.PhaseOpened
	case plc.StateClosing:
		return lanestatus.PhaseClosing
	case plc.StateAlarm:
		return lanestatus.PhaseError
	default:
		return lanestatus.PhaseIdle
	}
}
