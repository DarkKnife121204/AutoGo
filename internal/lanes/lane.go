package lanes

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"AutoGo/internal/lanestatus"
	"AutoGo/internal/scenarios"
)

const (
	pollInterval = 200 * time.Millisecond
	startTimeout = 10 * time.Second
)

type Lane struct {
	ID           string
	Name         string
	CheckpointID string
	Scenario     scenarios.Scenario

	mu            sync.Mutex
	mode          Mode
	activeVehicle *VehicleContext

	stopPoller chan struct{}
	pollerDone chan struct{}
}

func New(
	id string,
	name string,
	checkpointID string,
	scenario scenarios.Scenario,
	defaultMode Mode,
) (*Lane, error) {
	if id == "" {
		return nil, errors.New("ID линии не указан")
	}

	if scenario == nil {
		return nil, fmt.Errorf(
			"для линии %q не указан сценарий",
			id,
		)
	}

	return &Lane{
		ID:           id,
		Name:         name,
		CheckpointID: checkpointID,
		Scenario:     scenario,
		mode:         defaultMode,
	}, nil
}

func (l *Lane) ScenarioType() string {
	return l.Scenario.Type()
}

func (l *Lane) Mode() Mode {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.mode
}

func (l *Lane) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mode = ModeAutomatic

	return nil
}

func (l *Lane) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mode = ModeManual

	return nil
}

func (l *Lane) Reset() error {
	return l.Scenario.Reset()
}

func (l *Lane) Trigger(
	source scenarios.TriggerSource,
	plate string,
) error {
	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		return errors.New(
			"линия не в автоматическом режиме",
		)
	}

	if l.activeVehicle != nil {
		l.mu.Unlock()

		return errors.New(
			"линия занята: проезд уже выполняется",
		)
	}

	vehicle := &VehicleContext{
		ID:        newVehicleID(),
		Plate:     plate,
		Source:    string(source),
		Stage:     stageStarting,
		StartedAt: time.Now(),
	}

	l.activeVehicle = vehicle
	l.mu.Unlock()

	if err := l.Scenario.Trigger(source); err != nil {
		l.mu.Lock()
		if l.activeVehicle == vehicle {
			l.activeVehicle = nil
		}
		l.mu.Unlock()

		return err
	}

	return nil
}

func (l *Lane) Status() (lanestatus.LaneStatus, error) {
	status, err := l.Scenario.Status()
	if err != nil {
		return lanestatus.LaneStatus{}, err
	}

	status.LaneID = l.ID

	l.mu.Lock()
	status.Mode = string(l.mode)

	if l.activeVehicle != nil {
		vehicle := l.activeVehicle

		status.Busy = true
		status.Ready = false
		status.Vehicle = &lanestatus.VehicleInfo{
			ID:        vehicle.ID,
			Plate:     vehicle.Plate,
			Source:    vehicle.Source,
			Stage:     string(vehicle.Stage),
			StartedAt: vehicle.StartedAt.Unix(),
		}
	}
	l.mu.Unlock()

	return status, nil
}

func (l *Lane) StartPoller() {
	l.stopPoller = make(chan struct{})
	l.pollerDone = make(chan struct{})

	go func() {
		defer close(l.pollerDone)

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-l.stopPoller:
				return
			case <-ticker.C:
				l.poll()
			}
		}
	}()
}

func (l *Lane) StopPoller() {
	if l.stopPoller == nil {
		return
	}

	close(l.stopPoller)
	<-l.pollerDone
	l.stopPoller = nil
}

func (l *Lane) poll() {
	l.mu.Lock()
	active := l.activeVehicle != nil
	l.mu.Unlock()

	if !active {
		return
	}

	status, err := l.Scenario.Status()
	if err != nil {
		return
	}

	l.applyProgress(status.Phase, status.Alarm)
}

func (l *Lane) applyProgress(
	phase lanestatus.Phase,
	alarm bool,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	vehicle := l.activeVehicle
	if vehicle == nil {
		return
	}

	if alarm || phase == lanestatus.PhaseError {
		l.activeVehicle = nil

		return
	}

	switch vehicle.Stage {
	case stageStarting:
		if phase != lanestatus.PhaseIdle {
			vehicle.Stage = stageMoving

			return
		}

		if time.Since(vehicle.StartedAt) > startTimeout {
			l.activeVehicle = nil
		}

	case stageMoving:
		if phase == lanestatus.PhaseIdle {
			l.activeVehicle = nil
		}
	}
}
