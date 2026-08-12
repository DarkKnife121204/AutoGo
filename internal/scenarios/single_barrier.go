package scenarios

import (
	"errors"
	"sync"
	"time"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/plc"
)

const TypeSingleBarrier = "single_barrier"

const (
	defaultReleaseMode = "immediate"

	directionReverse = "reverse"

	singleStartTimeout = 10 * time.Second
)

type passageStage int

const (
	stageIdle passageStage = iota
	stageStarting
	stageMoving
)

type SingleBarrier struct {
	barrier *devices.Barrier

	releaseMode string

	mu        sync.Mutex
	stage     passageStage
	startedAt time.Time
}

func NewSingleBarrier(
	barrier *devices.Barrier,
	releaseMode string,
) (*SingleBarrier, error) {
	if barrier == nil {
		return nil, errors.New(
			"для сценария single_barrier не указан шлагбаум",
		)
	}

	if releaseMode == "" {
		releaseMode = defaultReleaseMode
	}

	return &SingleBarrier{
		barrier:     barrier,
		releaseMode: releaseMode,
	}, nil
}

func (s *SingleBarrier) Type() string {
	return TypeSingleBarrier
}

func (s *SingleBarrier) ReleaseMode() string {
	return s.releaseMode
}

func (s *SingleBarrier) Begin(direction string) error {
	s.mu.Lock()
	s.stage = stageStarting
	s.startedAt = time.Now()
	s.mu.Unlock()

	if direction == directionReverse {
		return s.barrier.StartReverse()
	}

	return s.barrier.Start()
}

func (s *SingleBarrier) Advance() (bool, error) {
	plcStatus, err := s.barrier.Status()
	if err != nil {
		return false, err
	}

	phase := phaseFromState(plcStatus.State)

	s.mu.Lock()
	defer s.mu.Unlock()

	if plcStatus.HasAlarm() || phase == lanestatus.PhaseError {
		s.stage = stageIdle

		return true, nil
	}

	switch s.stage {
	case stageStarting:
		if phase != lanestatus.PhaseIdle {
			s.stage = stageMoving

			return false, nil
		}

		if time.Since(s.startedAt) > singleStartTimeout {
			s.stage = stageIdle

			return true, nil
		}

	case stageMoving:
		if phase == lanestatus.PhaseIdle {
			s.stage = stageIdle

			return true, nil
		}
	}

	return false, nil
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

func (s *SingleBarrier) Confirm() error {
	return errors.New(
		"подтверждение неприменимо для сценария single_barrier",
	)
}

func (s *SingleBarrier) Reject() error {
	return errors.New(
		"отклонение неприменимо для сценария single_barrier",
	)
}

func (s *SingleBarrier) Snapshot() (lanestatus.Snapshot, error) {
	plcStatus, err := s.barrier.Status()
	if err != nil {
		return lanestatus.Snapshot{}, err
	}

	return lanestatus.Snapshot{
		Phase: phaseFromState(plcStatus.State),
		Alarm: plcStatus.HasAlarm(),
		Devices: map[string]lanestatus.DeviceStatus{
			s.barrier.ID: {
				Type:       "barrier",
				State:      plcStatus.State.String(),
				Controller: s.barrier.ControllerID,
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
