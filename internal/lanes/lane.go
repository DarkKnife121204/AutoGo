package lanes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"AutoGo/internal/access"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/scenarios"
)

const (
	pollInterval = 200 * time.Millisecond
	startTimeout = 10 * time.Second

	releaseModeImmediate            = "immediate"
	releaseModeExternalConfirmation = "external_confirmation"
)

type Lane struct {
	ID           string
	Name         string
	CheckpointID string
	Scenario     scenarios.Scenario

	releaseMode string
	decider     access.Decider

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
	decider access.Decider,
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
		releaseMode:  scenario.ReleaseMode(),
		decider:      decider,
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
	value string,
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
		Value:     value,
		Source:    string(source),
		StartedAt: time.Now(),
		source:    source,
	}

	if l.releaseMode == releaseModeExternalConfirmation {
		vehicle.Stage = stageWaiting
		l.activeVehicle = vehicle
		l.mu.Unlock()

		go l.decideAsync(vehicle)

		return nil
	}

	vehicle.Stage = stageStarting
	l.activeVehicle = vehicle
	l.mu.Unlock()

	if err := l.Scenario.Trigger(source); err != nil {
		l.clearVehicle(vehicle)

		return err
	}

	return nil
}

func (l *Lane) decideAsync(vehicle *VehicleContext) {
	decision, err := l.decider.Decide(
		context.Background(),
		access.Request{
			LaneID:     l.ID,
			Credential: vehicle.Value,
			Source:     vehicle.Source,
			Direction:  l.releaseMode,
		},
	)

	allowed := err == nil && decision.Allowed

	l.applyDecision(vehicle, allowed)
}

func (l *Lane) applyDecision(
	vehicle *VehicleContext,
	allowed bool,
) {
	l.mu.Lock()

	if l.activeVehicle != vehicle ||
		vehicle.Stage != stageWaiting {
		l.mu.Unlock()

		return
	}

	if !allowed {
		l.activeVehicle = nil
		l.mu.Unlock()

		return
	}

	vehicle.Stage = stageStarting
	vehicle.StartedAt = time.Now()
	source := vehicle.source
	l.mu.Unlock()

	if err := l.Scenario.Trigger(source); err != nil {
		l.clearVehicle(vehicle)
	}
}

func (l *Lane) clearVehicle(vehicle *VehicleContext) {
	l.mu.Lock()
	if l.activeVehicle == vehicle {
		l.activeVehicle = nil
	}
	l.mu.Unlock()
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

		if vehicle.Stage == stageWaiting {
			status.Phase = lanestatus.PhaseWaitingConfirmation
		}

		status.Vehicle = &lanestatus.VehicleInfo{
			ID:        vehicle.ID,
			Value:     vehicle.Value,
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
	physical := l.activeVehicle != nil &&
		l.activeVehicle.Stage != stageWaiting
	l.mu.Unlock()

	if !physical {
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
