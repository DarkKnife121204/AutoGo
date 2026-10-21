package lanes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"AutoGo/internal/access"
	"AutoGo/internal/axxon"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/plc"
	"AutoGo/internal/plcclient"
	"AutoGo/internal/scenarios"
	"AutoGo/internal/video"
)

const (
	pollInterval               = 200 * time.Millisecond
	photoCacheTimeout          = 600 * time.Second
	identificationCacheTimeout = 600 * time.Second
	validationCacheTimeout     = 660 * time.Second
	validationRequestTimeout   = 10 * time.Second
	plcResetInitTimeout        = 5 * time.Second

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
	services    map[string]*axxon.Client
	video       *video.Recorder

	mu                    sync.Mutex
	mode                  Mode
	stoppedFrom           lanestatus.State
	activeVehicle         *VehicleContext
	lastState             lanestatus.State
	lastNotifiedPLCStates map[string]string
	notifyQueue           chan access.ContextUpdate
	notifyDone            chan struct{}
	pending               []*VehicleContext
	queue                 []*VehicleContext
	queueDepth            int
	BidirectionalMode     bool

	cachedPlate          string
	cachedPlateAt        time.Time
	cachedPlateSource    scenarios.TriggerSource
	cachedPlateDirection string
	cachedKey            string
	cachedKeyAt          time.Time
	cachedKeySource      scenarios.TriggerSource
	cachedKeyDirection   string
	previousPLCStates    map[string]string
	expectedPLCInit      map[string]time.Time

	validationInProgress  int
	cachedValidation      access.Decision
	cachedValidationPlate string
	cachedValidationAt    time.Time

	photoVehicleID         string
	photo                  []byte
	photoCapturedAt        time.Time
	photoCaptureInProgress bool

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
	services map[string]*axxon.Client,
	queueDepth int,
	bidirectionalMode bool,
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
		ID:                    id,
		Name:                  name,
		CheckpointID:          checkpointID,
		Scenario:              scenario,
		releaseMode:           scenario.ReleaseMode(),
		decider:               decider,
		sources:               sources,
		services:              services,
		mode:                  defaultMode,
		queueDepth:            queueDepth,
		BidirectionalMode:     bidirectionalMode,
		previousPLCStates:     make(map[string]string),
		lastNotifiedPLCStates: make(map[string]string),
		expectedPLCInit:       make(map[string]time.Time),
		notifyQueue:           make(chan access.ContextUpdate, 64),
	}, nil
}

func (l *Lane) SetVideoRecorder(recorder *video.Recorder) {
	l.video = recorder
}

func (l *Lane) VideoStatus() (bool, time.Time, time.Time) {
	if l.video == nil {
		return false, time.Time{}, time.Time{}
	}
	return l.video.Status()
}

func (l *Lane) CurrentVideoFile() (string, error) {
	if l.video == nil {
		return "", video.ErrNotRecording
	}
	return l.video.CurrentFile()
}

func (l *Lane) ScenarioType() string {
	return l.Scenario.Type()
}

func (l *Lane) Mode() Mode {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.mode
}

var ErrManualCommandForbidden = errors.New("прямое управление запрещено: линия в automatic")

func (l *Lane) ManualBarrierCommand(barrier *devices.Barrier, command string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.mode != ModeManual {
		return ErrManualCommandForbidden
	}

	switch command {
	case "start":
		return barrier.Start()
	case "startreverse":
		return barrier.StartReverse()
	case "stop":
		return barrier.Stop()
	case "reset", "resetalarm":
		return barrier.Reset()
	case "open":
		return barrier.Open()
	case "close":
		return barrier.Close()
	default:
		return errors.New("неизвестная команда шлагбаума")
	}
}

func (l *Lane) Start() error {
	l.mu.Lock()
	err := l.startLocked()
	if err == nil {
		l.logStateChangeLocked()
	}
	l.mu.Unlock()
	if err != nil {
		return err
	}

	log.Printf("[lane %s] mode -> automatic", l.ID)
	return nil
}

func (l *Lane) startLocked() error {
	if l.activeVehicle != nil {
		switch l.stoppedFrom {
		case lanestatus.StateWaitingTransfer1,
			lanestatus.StateWaitingTransfer2,
			lanestatus.StateRollingBack:
			if err := l.Scenario.Resume(); err != nil {
				return err
			}
		}
	}

	l.mode = ModeAutomatic
	l.stoppedFrom = ""
	return nil
}

func (l *Lane) Stop() error {
	l.mu.Lock()

	if l.mode == ModeManual {
		l.mu.Unlock()
		return nil
	}

	snapshot, err := l.Scenario.Snapshot()
	if err != nil {
		log.Printf(
			"[lane %s] Stop: PLC status unavailable: %v",
			l.ID,
			err,
		)
		l.stoppedFrom = l.lastState
	} else {
		l.stoppedFrom = deriveState(snapshot, l.mode, l.activeVehicle)
	}

	l.mode = ModeManual
	if l.video != nil {
		l.video.StopSequence()
	}
	l.previousPLCStates = make(map[string]string)
	l.expectedPLCInit = make(map[string]time.Time)
	l.logStateChangeLocked()
	l.mu.Unlock()

	log.Printf("[lane %s] mode -> manual", l.ID)
	return nil
}

func (l *Lane) Reset() error {
	l.mu.Lock()

	snapshot, err := l.Scenario.Snapshot()
	if err != nil {
		l.mu.Unlock()
		return err
	}

	state := deriveState(snapshot, l.mode, l.activeVehicle)

	if state == lanestatus.StateStopping {
		err = l.startLocked()
		if err == nil {
			l.logStateChangeLocked()
		}
		l.mu.Unlock()
		return err
	}

	if state == lanestatus.StateIdentEntrance {
		l.activeVehicle = nil
		l.logStateChangeLocked()
		l.mu.Unlock()
		return nil
	}

	var resetIDs []string

	switch state {
	case lanestatus.StateNotReady:
		resetIDs, err = l.Scenario.RecoverNotReady()

	case lanestatus.StateWaitingTransfer1,
		lanestatus.StateWaitingTransfer2,
		lanestatus.StateConfirm,
		lanestatus.StateRollingBack,
		lanestatus.StateWaitingTransfer:
		err = l.Scenario.Cancel()

	default:
		resetIDs, err = l.Scenario.Reset()
	}

	if len(resetIDs) > 0 {
		deadline := time.Now().Add(plcResetInitTimeout)

		for _, id := range resetIDs {
			l.expectedPLCInit[id] = deadline
		}
	}

	if err != nil {
		if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) && l.activeVehicle != nil {
			l.activeVehicle.StartUncertain = true
		}

		l.mu.Unlock()
		return err
	}

	l.activeVehicle = nil
	l.logStateChangeLocked()
	l.mu.Unlock()

	return nil
}

func (l *Lane) Confirm() error {
	l.mu.Lock()

	if l.mode != ModeAutomatic {
		l.mu.Unlock()
		return errors.New("подтверждение недоступно: линия не в автоматическом режиме")
	}

	vehicle := l.activeVehicle
	if vehicle == nil || vehicle.StartUncertain {
		l.mu.Unlock()
		return errors.New("подтверждение недоступно: нет активной машины или результат PLC-команды неизвестен")
	}

	if !vehicle.validationCurrent() {
		l.mu.Unlock()
		return errors.New("подтверждение запрещено: разрешение Laravel отсутствует или истекло")
	}

	err := l.Scenario.Confirm()
	if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
		vehicle.StartUncertain = true
	}

	l.logStateChangeLocked()
	l.mu.Unlock()
	return err
}

func (l *Lane) Reject() error {
	l.mu.Lock()
	if l.mode != ModeAutomatic {
		l.mu.Unlock()
		return errors.New("отклонение недоступно: линия не в автоматическом режиме")
	}

	vehicle := l.activeVehicle
	if vehicle == nil || vehicle.StartUncertain {
		l.mu.Unlock()
		return errors.New("отклонение недоступно: нет активной машины или результат PLC-команды неизвестен")
	}

	err := l.Scenario.Reject()
	if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
		vehicle.StartUncertain = true
	}
	l.logStateChangeLocked()
	l.mu.Unlock()
	return err
}

func (l *Lane) trigger(
	source scenarios.TriggerSource,
	plate string,
	keyCode string,
	direction string,
) error {
	if (plate == "") == (keyCode == "") {
		return errors.New("должен быть указан либо plate, либо key_code")
	}

	if direction == "" {
		direction = "normal"
	}
	if direction != "normal" && direction != "reverse" {
		return errors.New("неизвестное направление движения")
	}
	if direction == "reverse" && !l.BidirectionalMode {
		return errors.New("обратное направление запрещено для этой линии")
	}

	l.mu.Lock()
	l.pruneExpiredQueueLocked()

	if l.mode == ModeManual {
		if l.isDuplicateLocked(source, plate, keyCode) {
			l.mu.Unlock()
			return errors.New("повторное событие по тому же идентификатору игнорируется")
		}
		if l.queueDepth <= 0 {
			l.cacheIdentificationLocked(source, plate, keyCode, direction)
			l.mu.Unlock()
			return nil
		}
		if len(l.pending)+len(l.queue) >= l.queueDepth {
			l.mu.Unlock()
			return errors.New("очередь заполнена")
		}

		l.cacheIdentificationLocked(source, plate, keyCode, direction)
		vehicle := l.newVehicleLocked(source, plate, keyCode, direction)
		l.pending = append(l.pending, vehicle)
		l.mu.Unlock()

		l.startPhotoCapture(vehicle)
		go l.decidePending(vehicle)

		log.Printf("[lane %s] pending in manual: value=%s", l.ID, vehicle.identity())
		return nil
	}

	if l.activeVehicle == nil && len(l.queue) == 0 && len(l.pending) == 0 {
		l.clearIdentificationCacheLocked()
		vehicle := l.newVehicleLocked(source, plate, keyCode, direction)
		vehicle.Waiting = true
		l.activeVehicle = vehicle
		l.photoVehicleID = vehicle.ID
		l.photoCaptureInProgress = false
		l.mu.Unlock()

		l.startPhotoCapture(vehicle)
		go l.decideAsync(vehicle)
		return nil
	}

	if l.isDuplicateLocked(source, plate, keyCode) {
		l.mu.Unlock()
		return errors.New("повторное событие по тому же идентификатору игнорируется")
	}
	if l.queueDepth <= 0 {
		l.cacheIdentificationLocked(source, plate, keyCode, direction)
		l.mu.Unlock()
		return nil
	}
	if len(l.pending)+len(l.queue) >= l.queueDepth {
		l.mu.Unlock()
		return errors.New("очередь заполнена")
	}

	l.cacheIdentificationLocked(source, plate, keyCode, direction)
	vehicle := l.newVehicleLocked(source, plate, keyCode, direction)
	l.pending = append(l.pending, vehicle)
	l.mu.Unlock()

	l.startPhotoCapture(vehicle)
	go l.decidePending(vehicle)

	log.Printf("[lane %s] pending (awaiting decision): value=%s", l.ID, vehicle.identity())
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
		Approved:  false,
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
	l.mu.Lock()
	l.validationInProgress++
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		l.validationInProgress--
		l.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), validationRequestTimeout)
	defer cancel()

	decision, err := l.decider.Decide(
		ctx,
		access.Request{
			LaneID:    l.ID,
			Plate:     vehicle.Plate,
			KeyCode:   vehicle.KeyCode,
			Source:    vehicle.Source,
			Direction: vehicle.Direction,
		},
	)

	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}

	if err != nil {
		l.mu.Lock()
		l.removePendingLocked(vehicle)
		l.promoteApprovedPendingLocked()
		l.mu.Unlock()
		log.Printf(
			"[lane %s] queue decision ERROR: value=%s error=%v",
			l.ID,
			vehicle.identity(),
			err,
		)

		return
	}

	allowed := decision.Allowed && (decision.PlateType == access.PlateTypeRegular || decision.PlateType == access.PlateTypeVIP)

	l.mu.Lock()

	l.cachedValidation = decision
	l.cachedValidationPlate = vehicle.Plate
	l.cachedValidationAt = time.Now()

	if !allowed {
		l.removePendingLocked(vehicle)
		l.promoteApprovedPendingLocked()

		l.mu.Unlock()

		log.Printf(
			"[lane %s] queue decision DENY: value=%s",
			l.ID, vehicle.identity(),
		)

		return
	}

	vehicle.Approved = true
	vehicle.PlateType = decision.PlateType
	vehicle.ValidatedAt = time.Now()

	l.promoteApprovedPendingLocked()

	l.mu.Unlock()

	log.Printf(
		"[lane %s] queue decision ALLOW: value=%s",
		l.ID, vehicle.identity(),
	)
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

func (l *Lane) pruneExpiredQueueLocked() {
	kept := l.queue[:0]

	for _, vehicle := range l.queue {
		if !vehicle.validationCurrent() {
			log.Printf(
				"[lane %s] queue validation expired: value=%s",
				l.ID,
				vehicle.identity(),
			)
			continue
		}

		kept = append(kept, vehicle)
	}

	l.queue = kept
}

func (l *Lane) takeQueueHeadLocked() *VehicleContext {
	l.pruneExpiredQueueLocked()

	if l.mode != ModeAutomatic ||
		l.activeVehicle != nil ||
		len(l.queue) == 0 {
		return nil
	}

	snapshot, err := l.Scenario.Snapshot()
	if err != nil {
		return nil
	}

	if !snapshot.Ready ||
		snapshot.Alarm ||
		(snapshot.Stage != "" && snapshot.Stage != "idle") {
		return nil
	}

	head := l.queue[0]
	if !head.validationCurrent() {
		return nil
	}

	l.queue = l.queue[1:]
	l.clearIdentificationCacheLocked()
	head.Waiting = true
	l.activeVehicle = head

	return head
}

func (l *Lane) beginVehicleLocked(vehicle *VehicleContext) error {
	log.Printf("[lane %s] begin: value=%s direction=%s", l.ID, vehicle.identity(), vehicle.Direction)

	if err := l.Scenario.Begin(vehicle.Direction); err != nil {
		if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
			vehicle.StartUncertain = true
			log.Printf("[lane %s] START outcome unknown, повтор запрещён до Reset: value=%s", l.ID, vehicle.identity())
		}
		log.Printf("[lane %s] begin FAILED: value=%s error=%v", l.ID, vehicle.identity(), err)
		return err
	}

	vehicle.Waiting = false
	l.photoVehicleID = vehicle.ID
	l.photoCaptureInProgress = vehicle.PhotoCaptureInProgress

	if len(vehicle.Photo) > 0 &&
		!vehicle.PhotoCapturedAt.IsZero() &&
		time.Since(vehicle.PhotoCapturedAt) < photoCacheTimeout {
		l.photo = vehicle.Photo
		l.photoCapturedAt = vehicle.PhotoCapturedAt
	}
	l.logStateChangeLocked()
	return nil
}

func (l *Lane) decideAsync(vehicle *VehicleContext) {
	l.mu.Lock()
	l.validationInProgress++
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		l.validationInProgress--
		l.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), validationRequestTimeout)
	defer cancel()

	decision, err := l.decider.Decide(
		ctx,
		access.Request{
			LaneID:    l.ID,
			Plate:     vehicle.Plate,
			KeyCode:   vehicle.KeyCode,
			Source:    vehicle.Source,
			Direction: vehicle.Direction,
		},
	)

	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}

	if err != nil {
		l.mu.Lock()

		if l.activeVehicle == vehicle && vehicle.Waiting {
			l.activeVehicle = nil
		}

		l.mu.Unlock()

		log.Printf(
			"[lane %s] decision ERROR: value=%s error=%v",
			l.ID,
			vehicle.identity(),
			err,
		)

		l.logStateChange()
		return
	}

	l.applyDecision(vehicle, decision)
}

func (l *Lane) applyDecision(vehicle *VehicleContext, decision access.Decision) {
	allowed := decision.Allowed && (decision.PlateType == access.PlateTypeRegular || decision.PlateType == access.PlateTypeVIP)

	log.Printf("[lane %s] decision: value=%s allowed=%v",
		l.ID, vehicle.identity(), allowed)

	l.mu.Lock()

	if l.activeVehicle != vehicle || !vehicle.Waiting {
		l.mu.Unlock()
		log.Printf("[lane %s] decision IGNORED (stale): value=%s",
			l.ID, vehicle.identity())
		return
	}

	l.cachedValidation = decision
	l.cachedValidationPlate = vehicle.Plate
	l.cachedValidationAt = time.Now()

	if !allowed {
		l.activeVehicle = nil
		l.mu.Unlock()
		return
	}

	vehicle.Approved = true
	vehicle.PlateType = decision.PlateType
	vehicle.ValidatedAt = time.Now()

	l.mu.Unlock()

	log.Printf("[lane %s] decision ALLOW stored, waiting for PLC ready: value=%s",
		l.ID, vehicle.identity())
}

func (l *Lane) clearVehicleLocked(vehicle *VehicleContext) {
	if l.activeVehicle == vehicle {
		l.activeVehicle = nil
		log.Printf("[lane %s] passage finished: value=%s", l.ID, vehicle.identity())
	}
}

func (l *Lane) Status() (lanestatus.LaneStatus, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

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

	return result, nil
}

func deriveState(
	snapshot lanestatus.Snapshot,
	mode Mode,
	vehicle *VehicleContext,
) lanestatus.State {
	if mode == ModeManual {
		return lanestatus.StateStopping
	}

	if (vehicle != nil && vehicle.StartUncertain) ||
		snapshot.Alarm || snapshot.Phase == lanestatus.PhaseError {
		return lanestatus.StateAlarm
	}

	if snapshot.Phase == lanestatus.PhaseNotReady {
		return lanestatus.StateNotReady
	}

	if snapshot.Stage == "idle" &&
		(vehicle == nil || vehicle.Waiting) {

		if snapshot.Ready {
			return lanestatus.StateIdentEntrance
		}

		return lanestatus.StateNotReady
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
	l.notifyDone = make(chan struct{})
	go l.notifyContextLoop()

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
	l.logStateChange()
}

func (l *Lane) StopPoller() {
	if l.stopPoller == nil {
		return
	}

	close(l.stopPoller)

	<-l.pollerDone
	<-l.notifyDone
	if l.video != nil {
		l.video.Close()
	}

	l.stopPoller = nil
}

func (l *Lane) poll() {
	l.mu.Lock()
	if l.mode != ModeAutomatic {
		l.mu.Unlock()
		l.logStateChange()
		return
	}

	l.pruneExpiredQueueLocked()

	if l.activeVehicle == nil || len(l.expectedPLCInit) > 0 {
		snapshot, err := l.Scenario.Snapshot()
		if err == nil {
			for id, device := range snapshot.Devices {
				if device.Type != "barrier" {
					continue
				}

				if deadline, expected := l.expectedPLCInit[id]; expected {
					if !time.Now().Before(deadline) {
						delete(l.expectedPLCInit, id)
					} else if device.State == plc.StateInit.String() {
						delete(l.expectedPLCInit, id)
						l.previousPLCStates[id] = device.State
						continue
					}
				}

				if l.activeVehicle != nil {
					continue
				}

				previous, exists := l.previousPLCStates[id]
				if exists &&
					device.State == plc.StateInit.String() &&
					previous != plc.StateInit.String() {
					if err := l.Scenario.StopInit(id); err != nil {
						log.Printf("[lane %s] stop after PLC Init failed: %v", l.ID, err)
					}
				}

				l.previousPLCStates[id] = device.State
			}

			if l.activeVehicle == nil &&
				len(l.expectedPLCInit) == 0 &&
				snapshot.Ready && !snapshot.Alarm &&
				(snapshot.Stage == "" || snapshot.Stage == "idle") {
				l.takeQueueHeadLocked()
				if l.queueDepth <= 0 &&
					l.activeVehicle == nil &&
					len(l.pending) == 0 &&
					len(l.queue) == 0 {
					if cached := l.takeCachedVehicleLocked(); cached != nil {
						l.activeVehicle = cached
						l.photoVehicleID = cached.ID
						l.photoCaptureInProgress = false
						l.mu.Unlock()
						l.startPhotoCapture(cached)
						go l.decideAsync(cached)
						l.logStateChange()
						return
					}
				}
			}
		}
	}
	vehicle := l.activeVehicle

	if vehicle == nil {
		l.mu.Unlock()
		l.logStateChange()
		return
	}

	if vehicle.StartUncertain {
		l.mu.Unlock()
		l.logStateChange()
		return
	}

	if vehicle.Waiting {
		if vehicle.Approved && !vehicle.validationCurrent() {

			log.Printf(
				"[lane %s] validation expired before Start: value=%s",
				l.ID,
				vehicle.identity(),
			)

			l.clearVehicleLocked(vehicle)
			l.mu.Unlock()
			l.logStateChange()
			return
		}

		if vehicle.Approved && !vehicle.StartUncertain &&
			len(l.expectedPLCInit) == 0 {
			snapshot, err := l.Scenario.Snapshot()
			if err != nil {
				log.Printf("[lane %s] PLC status error: %v", l.ID, err)
			} else if snapshot.Ready && !snapshot.Alarm &&
				(snapshot.Stage == "" || snapshot.Stage == "idle") {
				if err := l.beginVehicleLocked(vehicle); err != nil {
					log.Printf("[lane %s] vehicle start failed: %v", l.ID, err)
				}
			}
		}
		l.mu.Unlock()
		l.logStateChange()
		return
	}

	validationValid := vehicle.validationCurrent()

	done, err := l.Scenario.Advance(
		validationValid,
		vehicle.PlateType == access.PlateTypeVIP,
	)

	if err != nil {
		if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
			vehicle.StartUncertain = true
		}
		l.logStateChangeLocked()
		l.mu.Unlock()
		log.Printf("[lane %s] poll advance error: %v", l.ID, err)
		return
	}

	if done {
		l.clearVehicleLocked(vehicle)
	}

	l.logStateChangeLocked()
	l.mu.Unlock()
}

func (l *Lane) logStateChange() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logStateChangeLocked()
}

func (l *Lane) logStateChangeLocked() {
	snapshot, err := l.Scenario.Snapshot()
	if err != nil {
		return
	}

	state := deriveState(snapshot, l.mode, l.activeVehicle)
	prev := l.lastState
	changed := state != prev
	l.lastState = state

	plcChanged := false
	for id, device := range snapshot.Devices {
		if device.Type != "barrier" {
			continue
		}
		if old, exists := l.lastNotifiedPLCStates[id]; exists && old != device.State {
			plcChanged = true
		}
		l.lastNotifiedPLCStates[id] = device.State
	}

	if changed {
		log.Printf("[lane %s] state %s -> %s", l.ID, stateOrInitial(prev), state)
		if l.video != nil {
			if prev == lanestatus.StateConfirm && state != lanestatus.StateConfirm {
				l.video.StopSequence()
			}
			if prev != lanestatus.StateConfirm && state == lanestatus.StateConfirm {
				l.video.StartSequence()
			}
		}
	}

	if !changed && !plcChanged {
		return
	}

	if _, ok := l.decider.(interface {
		NotifyContext(context.Context, access.ContextUpdate) error
	}); !ok {
		return
	}

	direction := "normal"
	identification := ""
	if vehicle := l.activeVehicle; vehicle != nil {
		direction = vehicle.Direction
		identification = vehicle.Plate
		if identification == "" {
			identification = vehicle.KeyCode
		}
	}

	notifyState := state
	if state == lanestatus.StateWaitingTransfer {
		notifyState = lanestatus.StateWaitingTransfer1
	}

	update := access.ContextUpdate{
		LaneID:         l.ID,
		Direction:      direction,
		State:          string(notifyState),
		Identification: identification,
	}

	select {
	case l.notifyQueue <- update:
	default:
		log.Printf("[lane %s] очередь уведомлений Laravel заполнена: state=%s", l.ID, update.State)
	}
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
		return errors.New("next_state недоступен: линия не в автоматическом режиме")
	}

	vehicle := l.activeVehicle

	if vehicle == nil {
		l.mu.Unlock()
		return errors.New("next_state недоступен: нет активной машины")
	}

	if vehicle.StartUncertain {
		l.mu.Unlock()
		return errors.New("next_state запрещён: результат PLC-команды неизвестен; проверьте PLC и выполните Reset")
	}

	validationValid := vehicle.validationCurrent()

	done, err := l.Scenario.NextState(
		validationValid,
		vehicle.PlateType == access.PlateTypeVIP,
	)

	if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
		vehicle.StartUncertain = true
	}

	if err == nil && done {
		l.clearVehicleLocked(vehicle)
	}

	l.logStateChangeLocked()
	l.mu.Unlock()
	return err
}

func (l *Lane) PrevState() error {
	l.mu.Lock()
	if l.mode != ModeAutomatic {
		l.mu.Unlock()
		return errors.New("prev_state недоступен: линия не в автоматическом режиме")
	}

	vehicle := l.activeVehicle
	if vehicle == nil {
		l.mu.Unlock()
		return errors.New("prev_state недоступен: нет активной машины")
	}

	if vehicle.StartUncertain {
		l.mu.Unlock()
		return errors.New("prev_state запрещён: результат Start неизвестен; сначала проверить PLC и выполнить Reset")
	}

	validationValid := vehicle.validationCurrent()

	err := l.Scenario.PrevState(
		validationValid,
		vehicle.PlateType == access.PlateTypeVIP,
	)

	if errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
		vehicle.StartUncertain = true
	}

	l.logStateChangeLocked()
	l.mu.Unlock()
	return err
}

func (l *Lane) startPhotoCapture(
	vehicle *VehicleContext,
) {
	var camera *devices.Source

	for _, source := range l.sources {
		if source.Type != "camera" {
			continue
		}

		if source.Direction != vehicle.Direction {
			continue
		}

		camera = source
		break
	}

	if camera == nil {
		return
	}

	client, exists := l.services[camera.Service]
	if !exists {
		log.Printf(
			"[lane %s] photo capture skipped: Axxon service %q not found",
			l.ID,
			camera.Service,
		)

		return
	}

	l.mu.Lock()
	vehicle.PhotoCaptureInProgress = true

	if l.photoVehicleID == vehicle.ID {
		l.photoCaptureInProgress = true
	}

	l.mu.Unlock()

	go func() {
		image, err := client.Snapshot(
			context.Background(),
			camera.ExternalID,
		)

		capturedAt := time.Now()

		l.mu.Lock()

		vehicle.PhotoCaptureInProgress = false

		if err == nil {
			vehicle.Photo = image
			vehicle.PhotoCapturedAt = capturedAt
		}

		if l.photoVehicleID == vehicle.ID {
			l.photoCaptureInProgress = false

			if err == nil {
				l.photo = vehicle.Photo
				l.photoCapturedAt = vehicle.PhotoCapturedAt
			}
		}

		l.mu.Unlock()

		if err != nil {
			log.Printf(
				"[lane %s] photo capture FAILED: camera=%s value=%s error=%v",
				l.ID,
				camera.ID,
				vehicle.identity(),
				err,
			)

			return
		}

		log.Printf(
			"[lane %s] photo captured: camera=%s value=%s size=%d",
			l.ID,
			camera.ID,
			vehicle.identity(),
			len(image),
		)
	}()
}

func (l *Lane) Photo() (
	[]byte,
	time.Time,
	bool,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.photo) > 0 &&
		(l.photoCapturedAt.IsZero() ||
			time.Since(l.photoCapturedAt) >= photoCacheTimeout) {
		l.photo = nil
		l.photoCapturedAt = time.Time{}
	}

	var image []byte

	if len(l.photo) > 0 {
		image = append(
			[]byte(nil),
			l.photo...,
		)
	}

	return image,
		l.photoCapturedAt,
		l.photoCaptureInProgress
}

func (l *Lane) cacheIdentificationLocked(
	source scenarios.TriggerSource,
	plate string,
	keyCode string,
	direction string,
) {
	now := time.Now()

	if plate != "" {
		l.cachedPlate = plate
		l.cachedPlateAt = now
		l.cachedPlateSource = source
		l.cachedPlateDirection = direction
	}

	if keyCode != "" {
		l.cachedKey = keyCode
		l.cachedKeyAt = now
		l.cachedKeySource = source
		l.cachedKeyDirection = direction
	}
}

func (l *Lane) takeCachedVehicleLocked() *VehicleContext {
	now := time.Now()

	if l.cachedPlate != "" && now.Sub(l.cachedPlateAt) < identificationCacheTimeout {
		vehicle := l.newVehicleLocked(l.cachedPlateSource, l.cachedPlate, "", l.cachedPlateDirection)
		vehicle.Waiting = true
		l.clearIdentificationCacheLocked()
		return vehicle
	}

	if l.cachedKey != "" && now.Sub(l.cachedKeyAt) < identificationCacheTimeout {
		vehicle := l.newVehicleLocked(l.cachedKeySource, "", l.cachedKey, l.cachedKeyDirection)
		vehicle.Waiting = true
		l.clearIdentificationCacheLocked()
		return vehicle
	}

	l.clearIdentificationCacheLocked()
	return nil
}

func (l *Lane) clearIdentificationCacheLocked() {
	l.cachedPlate = ""
	l.cachedPlateAt = time.Time{}
	l.cachedPlateSource = ""
	l.cachedPlateDirection = ""
	l.cachedKey = ""
	l.cachedKeyAt = time.Time{}
	l.cachedKeySource = ""
	l.cachedKeyDirection = ""
}

func (l *Lane) ContextVehicle() (string, string, access.PlateType, time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	inProgress := l.validationInProgress > 0

	if l.activeVehicle == nil {
		return "", "", access.PlateTypeInvalid, time.Time{}, inProgress
	}

	vehicle := l.activeVehicle
	return vehicle.Plate, vehicle.Direction, vehicle.PlateType, vehicle.ValidatedAt, inProgress
}

func (l *Lane) CachedValidation() (access.Decision, string, time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cachedValidationAt.IsZero() ||
		time.Since(l.cachedValidationAt) >= validationCacheTimeout {
		return access.Decision{}, "", time.Time{}, false
	}

	return l.cachedValidation, l.cachedValidationPlate, l.cachedValidationAt, true
}

func (l *Lane) CachedIdentification() (
	string,
	time.Time,
	string,
	time.Time,
	string,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	if l.cachedPlate != "" &&
		now.Sub(l.cachedPlateAt) > identificationCacheTimeout {
		l.cachedPlate = ""
		l.cachedPlateAt = time.Time{}
	}

	if l.cachedKey != "" &&
		now.Sub(l.cachedKeyAt) > identificationCacheTimeout {
		l.cachedKey = ""
		l.cachedKeyAt = time.Time{}
	}

	direction := l.cachedPlateDirection

	if l.cachedPlate == "" {
		direction = l.cachedKeyDirection
	}

	return l.cachedPlate,
		l.cachedPlateAt,
		l.cachedKey,
		l.cachedKeyAt,
		direction
}

func (l *Lane) notifyContextLoop() {
	defer close(l.notifyDone)

	notifier, ok := l.decider.(interface {
		NotifyContext(context.Context, access.ContextUpdate) error
	})
	if !ok {
		log.Printf("[lane %s] Laravel context notifier недоступен", l.ID)
		return
	}

	const maxRetries = 3
	const retryDelay = time.Second

	for {
		select {
		case <-l.stopPoller:
			return
		case update := <-l.notifyQueue:
			for attempt := 0; attempt <= maxRetries; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err := notifier.NotifyContext(ctx, update)
				cancel()

				if err == nil {
					break
				}
				if attempt == maxRetries {
					log.Printf("[lane %s] Laravel context: state=%s, не отправлено после %d попыток: %v",
						l.ID, update.State, maxRetries+1, err)
					break
				}

				log.Printf("[lane %s] Laravel context: state=%s, попытка %d/%d неудачна: %v",
					l.ID, update.State, attempt+1, maxRetries+1, err)
				select {
				case <-l.stopPoller:
					return
				case <-time.After(retryDelay):
				}
			}
		}
	}
}
