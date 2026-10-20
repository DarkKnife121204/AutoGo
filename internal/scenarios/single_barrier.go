package scenarios

import (
	"AutoGo/internal/plcclient"
	"errors"
	"log"
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
	stageAlarm
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
	defer s.mu.Unlock()

	log.Printf(
		"[single] Begin: direction=%s barrier=%s",
		direction, s.barrier.ID,
	)

	if err := s.barrier.WaitReadyForStart(); err != nil {
		return err
	}

	var err error

	if direction == directionReverse {
		err = s.barrier.StartReverse()
	} else {
		err = s.barrier.Start()
	}

	if err != nil {
		return err
	}

	s.stage = stageStarting
	s.startedAt = time.Now()

	return nil
}

func (s *SingleBarrier) Advance(_ bool, _ bool) (bool, error) {
	plcStatus, err := s.barrier.Status()
	if err != nil {
		return false, err
	}

	phase := phaseFromState(plcStatus.State)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stage == stageAlarm {
		return false, nil
	}

	if plcStatus.HasAlarm() || phase == lanestatus.PhaseError {
		s.stage = stageAlarm

		return false, nil
	}

	if plcStatus.State == plc.StateClosed &&
		plcStatus.HasAlarmFlag(plc.AlarmNoEvent) {
		return false, nil
	}

	if plcStatus.State == plc.StateInit {
		return false, nil
	}

	if phase == lanestatus.PhaseNotReady {
		s.stage = stageAlarm
		return false, nil
	}

	switch s.stage {
	case stageStarting:
		if phase != lanestatus.PhaseIdle {
			s.stage = stageMoving

			return false, nil
		}

		if time.Since(s.startedAt) > singleStartTimeout {
			log.Printf("[single] PLC не начал операцию за %s", singleStartTimeout)
			s.stage = stageAlarm

			return false, nil
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

func (s *SingleBarrier) Reset() ([]string, error) {
	err := s.barrier.Reset()

	if err != nil {
		if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
			return []string{s.barrier.ID}, err
		}

		return nil, err
	}

	s.mu.Lock()
	s.stage = stageIdle
	s.startedAt = time.Time{}
	s.mu.Unlock()

	return []string{s.barrier.ID}, nil
}

func (s *SingleBarrier) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.barrier.Stop(); err != nil {
		return err
	}

	s.stage = stageIdle
	s.startedAt = time.Time{}

	return nil
}

func (s *SingleBarrier) RecoverNotReady() ([]string, error) {
	status, err := s.barrier.Status()
	if err != nil {
		return nil, err
	}

	var resetIDs []string

	switch {
	case status.State == plc.StateAlarm || status.HasAlarm():
		err = s.barrier.Reset()

		if err == nil || errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
			resetIDs = append(resetIDs, s.barrier.ID)
		}

	case status.State != plc.StateClosed && status.State != plc.StateInit:
		err = s.barrier.Stop()
	}

	if err != nil {
		return resetIDs, err
	}

	s.mu.Lock()
	s.stage = stageIdle
	s.startedAt = time.Time{}
	s.mu.Unlock()

	return resetIDs, nil
}

func (s *SingleBarrier) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.barrier.Status()
	if err != nil {
		return err
	}

	if status.State == plc.StateAlarm || status.HasAlarm() ||
		status.State == plc.StateInit {
		return errors.New("возврат из Stopping запрещён: PLC не готов")
	}

	if s.stage == stageAlarm {
		return errors.New("возврат из Stopping запрещён: сценарий в Alarm")
	}

	if s.stage == stageStarting && status.State == plc.StateClosed {
		s.startedAt = time.Now()
	}

	return nil
}

func (s *SingleBarrier) StopInit(barrierID string) error {
	if s.barrier.ID != barrierID {
		return errors.New("шлагбаум не принадлежит линии")
	}
	return s.barrier.Stop()
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

	s.mu.Lock()
	alarm := s.stage == stageAlarm ||
		plcStatus.HasAlarm() ||
		plcStatus.State == plc.StateAlarm
	s.mu.Unlock()

	phase := phaseFromState(plcStatus.State)
	if alarm {
		phase = lanestatus.PhaseError
	}

	return lanestatus.Snapshot{
		Phase: phase,
		Alarm: alarm,
		Ready: !alarm && plcStatus.State == plc.StateClosed,
		Devices: map[string]lanestatus.DeviceStatus{
			s.barrier.ID: {
				Type:       "barrier",
				State:      plcStatus.State.String(),
				Controller: s.barrier.ControllerID,
			},
		},
	}, nil
}

func (s *SingleBarrier) NextState(_ bool, _ bool) (bool, error) {
	return false, errors.New(
		"next_state неприменим для сценария single_barrier",
	)
}

func (s *SingleBarrier) PrevState(_ bool, _ bool) error {
	return errors.New(
		"prev_state неприменим для сценария single_barrier",
	)
}

func phaseFromState(state plc.State) lanestatus.Phase {
	switch state {
	case plc.StateClosed:
		return lanestatus.PhaseIdle
	case plc.StateOpening:
		return lanestatus.PhaseOpening
	case plc.StateOpened:
		return lanestatus.PhaseOpened
	case plc.StateClosing:
		return lanestatus.PhaseClosing
	case plc.StateAlarm:
		return lanestatus.PhaseError
	default:
		return lanestatus.PhaseNotReady
	}
}
