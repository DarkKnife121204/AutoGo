package lanes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"AutoGo/internal/access"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/scenarios"
)

const (
	pollInterval = 200 * time.Millisecond

	releaseModeExternalConfirmation = "external_confirmation"
)

type Lane struct {
	ID           string
	Name         string
	CheckpointID string
	Scenario     scenarios.Scenario

	releaseMode string
	decider     access.Decider
	sources     []*devices.Source

	mu            sync.Mutex
	mode          Mode
	activeVehicle *VehicleContext
	lastState     lanestatus.State
	pending       []*VehicleContext
	queue         []*VehicleContext
	queueDepth    int

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
	sources []*devices.Source,
	queueDepth int,
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
		sources:      sources,
		mode:         defaultMode,
		queueDepth:   queueDepth,
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

	log.Printf("[lane %s] mode -> automatic", l.ID)

	return nil
}

func (l *Lane) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mode = ModeManual

	log.Printf("[lane %s] mode -> manual", l.ID)

	return nil
}

func (l *Lane) Reset() error {
	return l.Scenario.Reset()
}

func (l *Lane) Confirm() error {
	err := l.Scenario.Confirm()
	l.logStateChange()

	return err
}

func (l *Lane) Reject() error {
	err := l.Scenario.Reject()
	l.logStateChange()

	return err
}

func (l *Lane) Trigger(
	source scenarios.TriggerSource,
	value string,
	direction string,
) error {
	log.Printf(
		"[lane %s] trigger: source=%s value=%s direction=%s",
		l.ID, source, value, direction,
	)

	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		return errors.New(
			"линия не в автоматическом режиме",
		)
	}

	if l.activeVehicle == nil {
		vehicle := l.newVehicleLocked(source, value, direction)

		if l.releaseMode == releaseModeExternalConfirmation {
			vehicle.Waiting = true
			l.activeVehicle = vehicle
			l.mu.Unlock()

			go l.decideAsync(vehicle)

			return nil
		}

		l.activeVehicle = vehicle
		l.mu.Unlock()

		log.Printf(
			"[lane %s] begin (immediate): value=%s direction=%s",
			l.ID, vehicle.Value, vehicle.Direction,
		)

		if err := l.Scenario.Begin(vehicle.Direction); err != nil {
			log.Printf(
				"[lane %s] begin (immediate) FAILED: value=%s error=%v",
				l.ID, vehicle.Value, err,
			)
			l.clearVehicle(vehicle)

			return err
		}

		return nil
	}

	if l.isDuplicateLocked(source, value) {
		l.mu.Unlock()

		return errors.New(
			"повторное событие по тому же идентификатору игнорируется",
		)
	}

	if l.queueDepth <= 0 {
		l.mu.Unlock()

		return errors.New(
			"линия занята: проезд уже выполняется",
		)
	}

	if len(l.pending)+len(l.queue) >= l.queueDepth {
		l.mu.Unlock()

		return errors.New(
			"очередь заполнена",
		)
	}

	vehicle := l.newVehicleLocked(source, value, direction)

	if l.releaseMode != releaseModeExternalConfirmation {
		l.queue = append(l.queue, vehicle)
		l.mu.Unlock()

		log.Printf(
			"[lane %s] queued (immediate): value=%s",
			l.ID, vehicle.Value,
		)

		return nil
	}

	l.pending = append(l.pending, vehicle)
	l.mu.Unlock()

	log.Printf(
		"[lane %s] pending (awaiting decision): value=%s",
		l.ID, vehicle.Value,
	)

	go l.decidePending(vehicle)

	return nil
}

func (l *Lane) newVehicleLocked(
	source scenarios.TriggerSource,
	value string,
	direction string,
) *VehicleContext {
	if direction == "" {
		direction = "normal"
	}

	return &VehicleContext{
		ID:        newVehicleID(),
		Value:     value,
		Source:    string(source),
		Direction: direction,
		StartedAt: time.Now(),
		source:    source,
	}
}

func (l *Lane) isDuplicateLocked(
	source scenarios.TriggerSource,
	value string,
) bool {
	if value == "" {
		return false
	}

	src := string(source)

	if l.activeVehicle != nil &&
		l.activeVehicle.Value == value &&
		l.activeVehicle.Source == src {
		return true
	}

	for _, v := range l.pending {
		if v.Value == value && v.Source == src {
			return true
		}
	}

	for _, v := range l.queue {
		if v.Value == value && v.Source == src {
			return true
		}
	}

	return false
}

func (l *Lane) decidePending(vehicle *VehicleContext) {
	decision, err := l.decider.Decide(
		context.Background(),
		access.Request{
			LaneID:     l.ID,
			Credential: vehicle.Value,
			Source:     vehicle.Source,
			Direction:  vehicle.Direction,
		},
	)

	allowed := err == nil && decision.Allowed

	l.mu.Lock()

	l.removePendingLocked(vehicle)

	if !allowed {
		l.mu.Unlock()

		log.Printf(
			"[lane %s] queue decision DENY: value=%s",
			l.ID, vehicle.Value,
		)

		return
	}

	l.queue = append(l.queue, vehicle)

	next := l.takeQueueHeadLocked()
	l.mu.Unlock()

	log.Printf(
		"[lane %s] queue decision ALLOW: value=%s",
		l.ID, vehicle.Value,
	)

	if next != nil {
		log.Printf(
			"[lane %s] activating from queue: value=%s",
			l.ID, next.Value,
		)
		l.beginVehicle(next)
	}
}

func (l *Lane) removePendingLocked(vehicle *VehicleContext) {
	for i, v := range l.pending {
		if v == vehicle {
			l.pending = append(l.pending[:i], l.pending[i+1:]...)

			return
		}
	}
}

func (l *Lane) takeQueueHeadLocked() *VehicleContext {
	if l.activeVehicle != nil || len(l.queue) == 0 {
		return nil
	}

	head := l.queue[0]
	l.queue = l.queue[1:]
	l.activeVehicle = head

	return head
}

func (l *Lane) beginVehicle(vehicle *VehicleContext) {
	log.Printf(
		"[lane %s] begin: value=%s direction=%s",
		l.ID, vehicle.Value, vehicle.Direction,
	)

	if err := l.Scenario.Begin(vehicle.Direction); err != nil {
		log.Printf(
			"[lane %s] begin FAILED: value=%s error=%v",
			l.ID, vehicle.Value, err,
		)

		l.clearVehicle(vehicle)

		return
	}

	l.logStateChange()
}

func (l *Lane) decideAsync(vehicle *VehicleContext) {
	decision, err := l.decider.Decide(
		context.Background(),
		access.Request{
			LaneID:     l.ID,
			Credential: vehicle.Value,
			Source:     vehicle.Source,
			Direction:  vehicle.Direction,
		},
	)

	allowed := err == nil && decision.Allowed

	l.applyDecision(vehicle, allowed)
}

func (l *Lane) applyDecision(
	vehicle *VehicleContext,
	allowed bool,
) {
	log.Printf(
		"[lane %s] decision: value=%s allowed=%v",
		l.ID, vehicle.Value, allowed,
	)

	l.mu.Lock()

	if l.activeVehicle != vehicle || !vehicle.Waiting {
		log.Printf(
			"[lane %s] decision IGNORED (stale): value=%s",
			l.ID, vehicle.Value,
		)
		l.mu.Unlock()

		return
	}

	if !allowed {
		l.activeVehicle = nil

		next := l.takeQueueHeadLocked()
		l.mu.Unlock()

		if next != nil {
			l.beginVehicle(next)
		}

		return
	}

	vehicle.Waiting = false
	l.mu.Unlock()

	l.beginVehicle(vehicle)
}

func (l *Lane) clearVehicle(vehicle *VehicleContext) {
	l.mu.Lock()

	if l.activeVehicle == vehicle {
		l.activeVehicle = nil
	}

	next := l.takeQueueHeadLocked()
	l.mu.Unlock()

	log.Printf(
		"[lane %s] passage finished: value=%s",
		l.ID, vehicle.Value,
	)

	if next != nil {
		log.Printf(
			"[lane %s] activating from queue: value=%s",
			l.ID, next.Value,
		)
		l.beginVehicle(next)
	}
}

func (l *Lane) Status() (lanestatus.LaneStatus, error) {
	snapshot, err := l.Scenario.Snapshot()
	if err != nil {
		log.Printf("[lane %s] snapshot error: %v", l.ID, err)
		return lanestatus.LaneStatus{}, err
	}

	deviceMap := make(
		map[string]lanestatus.DeviceStatus,
		len(snapshot.Devices)+len(l.sources),
	)

	for id, d := range snapshot.Devices {
		deviceMap[id] = d
	}

	for _, source := range l.sources {
		deviceMap[source.ID] = lanestatus.DeviceStatus{
			Type:       source.Type,
			ExternalID: source.ExternalID,
		}
	}

	result := lanestatus.LaneStatus{
		LaneID:      l.ID,
		Scenario:    l.Scenario.Type(),
		ReleaseMode: l.releaseMode,
		Devices:     deviceMap,
	}

	l.mu.Lock()
	mode := l.mode
	vehicle := l.activeVehicle

	if vehicle != nil {
		stage := "active"
		if vehicle.Waiting {
			stage = "waiting"
		}

		result.Vehicle = &lanestatus.VehicleInfo{
			ID:        vehicle.ID,
			Value:     vehicle.Value,
			Direction: vehicle.Direction,
			Source:    vehicle.Source,
			Stage:     stage,
			StartedAt: vehicle.StartedAt.Unix(),
		}
	}

	for _, v := range l.queue {
		result.Queue = append(result.Queue, lanestatus.QueuedInfo{
			ID:        v.ID,
			Value:     v.Value,
			Source:    v.Source,
			StartedAt: v.StartedAt.Unix(),
		})
	}

	result.State = deriveState(snapshot, mode, vehicle)
	l.mu.Unlock()

	return result, nil
}

func deriveState(
	snapshot lanestatus.Snapshot,
	mode Mode,
	vehicle *VehicleContext,
) lanestatus.State {
	if snapshot.Alarm || snapshot.Phase == lanestatus.PhaseError {
		return lanestatus.StateAlarm
	}

	if mode == ModeManual {
		return lanestatus.StateStopping
	}

	switch snapshot.Stage {
	case "entry":
		return lanestatus.StateWaitingTransfer1
	case "confirm":
		return lanestatus.StateConfirm
	case "exit":
		return lanestatus.StateWaitingTransfer2
	case "reject":
		return lanestatus.StateRollingBack
	}

	if vehicle != nil && !vehicle.Waiting {
		return lanestatus.StateWaitingTransfer
	}

	if snapshot.Phase == lanestatus.PhaseOpening ||
		snapshot.Phase == lanestatus.PhaseOpened ||
		snapshot.Phase == lanestatus.PhaseClosing {
		return lanestatus.StateWaitingTransfer
	}

	return lanestatus.StateIdentEntrance
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
	vehicle := l.activeVehicle
	physical := vehicle != nil && !vehicle.Waiting
	l.mu.Unlock()

	if !physical {
		return
	}

	done, err := l.Scenario.Advance()
	if err != nil {
		log.Printf("[lane %s] poll advance error: %v", l.ID, err)

		return
	}

	if done {
		l.clearVehicle(vehicle)
	}

	l.logStateChange()
}

func (l *Lane) logStateChange() {
	snapshot, err := l.Scenario.Snapshot()
	if err != nil {
		return
	}

	l.mu.Lock()
	state := deriveState(snapshot, l.mode, l.activeVehicle)

	if state != l.lastState {
		prev := l.lastState
		l.lastState = state
		l.mu.Unlock()

		log.Printf(
			"[lane %s] state %s -> %s",
			l.ID, stateOrInitial(prev), state,
		)

		return
	}
	l.mu.Unlock()
}

func stateOrInitial(s lanestatus.State) lanestatus.State {
	if s == "" {
		return "—"
	}

	return s
}
