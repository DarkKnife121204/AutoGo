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

	l.mode = ModeAutomatic

	current := l.activeVehicle

	var next *VehicleContext

	if current == nil {
		next = l.takeQueueHeadLocked()
	}

	resume := current != nil &&
		current.Waiting &&
		current.Approved

	if resume {
		current.Waiting = false
	}

	l.mu.Unlock()

	log.Printf("[lane %s] mode -> automatic", l.ID)

	l.logStateChange()

	if resume {
		log.Printf(
			"[lane %s] activating approved vehicle after start: value=%s",
			l.ID, current.identity(),
		)

		l.beginVehicle(current)

		return nil
	}

	if next != nil {
		log.Printf(
			"[lane %s] activating from queue after start: value=%s",
			l.ID, next.identity(),
		)

		l.beginVehicle(next)
	}

	return nil
}

func (l *Lane) Stop() error {
	l.mu.Lock()
	l.mode = ModeManual
	l.mu.Unlock()

	log.Printf("[lane %s] mode -> manual", l.ID)

	l.logStateChange()

	return nil
}

func (l *Lane) Reset() error {
	l.mu.Lock()

	if err := l.Scenario.Reset(); err != nil {
		l.mu.Unlock()
		return err
	}

	l.activeVehicle = nil

	var next *VehicleContext

	if l.mode == ModeAutomatic {
		next = l.takeQueueHeadLocked()
	}

	l.mu.Unlock()

	if next != nil {
		log.Printf(
			"[lane %s] activating from queue after reset: value=%s",
			l.ID, next.identity(),
		)

		l.beginVehicle(next)
	}

	return nil
}

func (l *Lane) Confirm() error {
	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		return errors.New(
			"подтверждение недоступно: линия не в автоматическом режиме",
		)
	}

	l.mu.Unlock()

	err := l.Scenario.Confirm()
	l.logStateChange()

	return err
}

func (l *Lane) Reject() error {
	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		return errors.New(
			"отклонение недоступно: линия не в автоматическом режиме",
		)
	}

	l.mu.Unlock()

	err := l.Scenario.Reject()
	l.logStateChange()

	return err
}

func (l *Lane) trigger(
	source scenarios.TriggerSource,
	plate string,
	keyCode string,
	direction string,
) error {
	if (plate == "") == (keyCode == "") {
		return errors.New(
			"должен быть указан либо plate, либо key_code",
		)
	}

	l.mu.Lock()

	if l.mode == ModeManual {
		if l.isDuplicateLocked(source, plate, keyCode) {
			l.mu.Unlock()

			return errors.New(
				"повторное событие по тому же идентификатору игнорируется",
			)
		}

		if l.queueDepth <= 0 {
			l.mu.Unlock()

			return errors.New(
				"очередь отключена для линии",
			)
		}

		if len(l.pending)+len(l.queue) >= l.queueDepth {
			l.mu.Unlock()

			return errors.New(
				"очередь заполнена",
			)
		}

		vehicle := l.newVehicleLocked(source, plate, keyCode, direction)

		if l.releaseMode != releaseModeExternalConfirmation {
			l.queue = append(l.queue, vehicle)
			l.mu.Unlock()

			log.Printf(
				"[lane %s] queued in manual: value=%s",
				l.ID,
				vehicle.identity(),
			)

			return nil
		}

		l.pending = append(l.pending, vehicle)
		l.mu.Unlock()

		log.Printf(
			"[lane %s] pending in manual: value=%s",
			l.ID,
			vehicle.identity(),
		)

		go l.decidePending(vehicle)

		return nil
	}

	if l.activeVehicle == nil {
		vehicle := l.newVehicleLocked(source, plate, keyCode, direction)

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
			l.ID, vehicle.identity(), vehicle.Direction,
		)

		if err := l.Scenario.Begin(vehicle.Direction); err != nil {
			log.Printf(
				"[lane %s] begin (immediate) FAILED: value=%s error=%v",
				l.ID, vehicle.identity(), err,
			)

			return err
		}

		return nil
	}

	if l.isDuplicateLocked(source, plate, keyCode) {
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

	vehicle := l.newVehicleLocked(source, plate, keyCode, direction)

	if l.releaseMode != releaseModeExternalConfirmation {
		l.queue = append(l.queue, vehicle)
		l.mu.Unlock()

		log.Printf(
			"[lane %s] queued (immediate): value=%s",
			l.ID, vehicle.identity(),
		)

		return nil
	}

	l.pending = append(l.pending, vehicle)
	l.mu.Unlock()

	log.Printf(
		"[lane %s] pending (awaiting decision): value=%s",
		l.ID, vehicle.identity(),
	)

	go l.decidePending(vehicle)

	return nil
}

func (l *Lane) newVehicleLocked(
	source scenarios.TriggerSource,
	plate string,
	keyCode string,
	direction string,
) *VehicleContext {
	if direction == "" {
		direction = "normal"
	}

	return &VehicleContext{
		ID:        newVehicleID(),
		Plate:     plate,
		KeyCode:   keyCode,
		Source:    string(source),
		Direction: direction,
		StartedAt: time.Now(),
		source:    source,
	}
}

func (l *Lane) isDuplicateLocked(
	source scenarios.TriggerSource,
	plate string,
	keyCode string,
) bool {
	same := func(v *VehicleContext) bool {
		return v.Source == string(source) &&
			v.Plate == plate &&
			v.KeyCode == keyCode
	}

	if l.activeVehicle != nil && same(l.activeVehicle) {
		return true
	}

	for _, v := range l.pending {
		if same(v) {
			return true
		}
	}

	for _, v := range l.queue {
		if same(v) {
			return true
		}
	}

	return false
}

func (l *Lane) decidePending(vehicle *VehicleContext) {
	decision, err := l.decider.Decide(
		context.Background(),
		access.Request{
			LaneID:    l.ID,
			Plate:     vehicle.Plate,
			KeyCode:   vehicle.KeyCode,
			Source:    vehicle.Source,
			Direction: vehicle.Direction,
		},
	)

	if err != nil {
		log.Printf(
			"[lane %s] queue decision ERROR: value=%s error=%v",
			l.ID,
			vehicle.identity(),
			err,
		)

		return
	}

	allowed := decision.Allowed

	l.mu.Lock()

	if !allowed {
		l.removePendingLocked(vehicle)
		l.promoteApprovedPendingLocked()

		next := l.takeQueueHeadLocked()
		l.mu.Unlock()

		log.Printf(
			"[lane %s] queue decision DENY: value=%s",
			l.ID, vehicle.identity(),
		)

		if next != nil {
			l.beginVehicle(next)
		}

		return
	}

	vehicle.Approved = true

	l.promoteApprovedPendingLocked()

	next := l.takeQueueHeadLocked()
	l.mu.Unlock()

	log.Printf(
		"[lane %s] queue decision ALLOW: value=%s",
		l.ID, vehicle.identity(),
	)

	if next != nil {
		log.Printf(
			"[lane %s] activating from queue: value=%s",
			l.ID, next.identity(),
		)

		l.beginVehicle(next)
	}
}

func (l *Lane) promoteApprovedPendingLocked() {
	for len(l.pending) > 0 && l.pending[0].Approved {
		vehicle := l.pending[0]
		l.pending = l.pending[1:]

		l.queue = append(l.queue, vehicle)
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
	if l.mode != ModeAutomatic {
		return nil
	}

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
		l.ID, vehicle.identity(), vehicle.Direction,
	)

	if err := l.Scenario.Begin(vehicle.Direction); err != nil {
		log.Printf(
			"[lane %s] begin FAILED: value=%s error=%v",
			l.ID, vehicle.identity(), err,
		)

		return
	}

	l.logStateChange()
}

func (l *Lane) decideAsync(vehicle *VehicleContext) {
	decision, err := l.decider.Decide(
		context.Background(),
		access.Request{
			LaneID:    l.ID,
			Plate:     vehicle.Plate,
			KeyCode:   vehicle.KeyCode,
			Source:    vehicle.Source,
			Direction: vehicle.Direction,
		},
	)

	if err != nil {
		log.Printf(
			"[lane %s] decision ERROR: value=%s error=%v",
			l.ID,
			vehicle.identity(),
			err,
		)

		return
	}

	l.applyDecision(vehicle, decision.Allowed)
}

func (l *Lane) applyDecision(
	vehicle *VehicleContext,
	allowed bool,
) {
	log.Printf(
		"[lane %s] decision: value=%s allowed=%v",
		l.ID, vehicle.identity(), allowed,
	)

	l.mu.Lock()

	if l.activeVehicle != vehicle || !vehicle.Waiting {
		log.Printf(
			"[lane %s] decision IGNORED (stale): value=%s",
			l.ID, vehicle.identity(),
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

	vehicle.Approved = true

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		log.Printf(
			"[lane %s] decision ALLOW stored, waiting for automatic mode: value=%s",
			l.ID, vehicle.identity(),
		)

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
		l.ID, vehicle.identity(),
	)

	if next != nil {
		log.Printf(
			"[lane %s] activating from queue: value=%s",
			l.ID, next.identity(),
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
			Plate:     vehicle.Plate,
			KeyCode:   vehicle.KeyCode,
			Direction: vehicle.Direction,
			Source:    vehicle.Source,
			Stage:     stage,
			StartedAt: vehicle.StartedAt.Unix(),
		}
	}

	for _, v := range l.queue {
		result.Queue = append(result.Queue, lanestatus.QueuedInfo{
			ID:        v.ID,
			Plate:     v.Plate,
			KeyCode:   v.KeyCode,
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

	if !snapshot.Ready {
		return lanestatus.StateNotReady
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

	if l.mode != ModeAutomatic {
		l.mu.Unlock()
		return
	}

	vehicle := l.activeVehicle
	physical := vehicle != nil && !vehicle.Waiting

	l.mu.Unlock()

	if !physical {
		l.logStateChange()
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

func (l *Lane) TriggerPlate(
	source scenarios.TriggerSource,
	plate string,
	direction string,
) error {
	return l.trigger(
		source,
		plate,
		"",
		direction,
	)
}

func (l *Lane) TriggerKeyCode(
	source scenarios.TriggerSource,
	keyCode string,
	direction string,
) error {
	return l.trigger(
		source,
		"",
		keyCode,
		direction,
	)
}

func (l *Lane) NextState() error {
	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		return errors.New(
			"next_state недоступен: линия не в автоматическом режиме",
		)
	}

	vehicle := l.activeVehicle

	if vehicle == nil {
		l.mu.Unlock()

		return errors.New(
			"next_state недоступен: нет активной машины",
		)
	}

	l.mu.Unlock()

	done, err := l.Scenario.NextState()
	if err != nil {
		return err
	}

	if done {
		l.clearVehicle(vehicle)
	}

	l.logStateChange()

	return nil
}

func (l *Lane) PrevState() error {
	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()

		return errors.New(
			"prev_state недоступен: линия не в автоматическом режиме",
		)
	}

	if l.activeVehicle == nil {
		l.mu.Unlock()

		return errors.New(
			"prev_state недоступен: нет активной машины",
		)
	}

	l.mu.Unlock()

	if err := l.Scenario.PrevState(); err != nil {
		return err
	}

	l.logStateChange()

	return nil
}
