package scenarios

import (
	"AutoGo/internal/plcclient"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanestatus"
	"AutoGo/internal/plc"
)

const TypeDoubleBarrier = "double_barrier"

const doubleStartTimeout = 10 * time.Second

type gateStage int

const (
	gateIdle gateStage = iota
	gateEntry
	gateEntryHold
	gateConfirm
	gateExit
	gateReject
	gateAlarm
)

type DoubleBarrier struct {
	entry *devices.Barrier
	exit  *devices.Barrier

	releaseMode string

	mu               sync.Mutex
	stage            gateStage
	alarmPrevious    gateStage
	startedAt        time.Time
	currentDirection string
}

func NewDoubleBarrier(
	entry *devices.Barrier,
	exit *devices.Barrier,
	releaseMode string,
) (*DoubleBarrier, error) {
	if entry == nil || exit == nil {
		return nil, errors.New(
			"для сценария double_barrier нужны оба шлагбаума",
		)
	}

	if releaseMode == "" {
		releaseMode = defaultReleaseMode
	}

	return &DoubleBarrier{
		entry:       entry,
		exit:        exit,
		releaseMode: releaseMode,
	}, nil
}

func (d *DoubleBarrier) Type() string {
	return TypeDoubleBarrier
}

func (d *DoubleBarrier) ReleaseMode() string {
	return d.releaseMode
}

func (d *DoubleBarrier) inbound() *devices.Barrier {
	if d.currentDirection == directionReverse {
		return d.exit
	}

	return d.entry
}

func (d *DoubleBarrier) outbound() *devices.Barrier {
	if d.currentDirection == directionReverse {
		return d.entry
	}

	return d.exit
}

func (d *DoubleBarrier) enterAlarmLocked() {
	if d.stage != gateAlarm {
		d.alarmPrevious = d.stage
	}

	d.stage = gateAlarm
	d.startedAt = time.Time{}
}

func (d *DoubleBarrier) Begin(direction string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.waitReadyForStart(); err != nil {
		return err
	}

	d.currentDirection = direction
	inbound := d.inbound()

	log.Printf(
		"[gate] Begin: direction=%s inbound=%s",
		direction, inbound.ID,
	)

	if err := inbound.Start(); err != nil {
		d.currentDirection = ""

		log.Printf("[gate] Begin Start error: %v", err)

		return err
	}

	d.stage = gateEntry
	d.alarmPrevious = gateIdle
	d.startedAt = time.Now()

	return nil
}

func (d *DoubleBarrier) Confirm() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stage != gateConfirm {
		return errors.New(
			"подтверждение недоступно: машина не в шлюзе",
		)
	}

	if err := d.waitReadyForStart(); err != nil {
		return err
	}

	outbound := d.outbound()

	log.Printf("[gate] Confirm: outbound=%s", outbound.ID)

	if err := outbound.Start(); err != nil {
		return err
	}

	d.stage = gateExit
	d.startedAt = time.Now()

	return nil
}

func (d *DoubleBarrier) Reject() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stage != gateConfirm {
		return errors.New(
			"отклонение недоступно: машина не в шлюзе",
		)
	}

	if err := d.waitReadyForStart(); err != nil {
		return err
	}

	inbound := d.inbound()

	log.Printf("[gate] Reject: inbound=%s", inbound.ID)

	if err := inbound.Start(); err != nil {
		return err
	}

	d.stage = gateReject
	d.startedAt = time.Now()

	return nil
}

func (d *DoubleBarrier) Advance(validationValid bool, vip bool) (bool, error) {
	d.mu.Lock()
	stage := d.stage
	inbound := d.inbound()
	outbound := d.outbound()
	d.mu.Unlock()

	switch stage {
	case gateEntryHold:
		return false, nil

	case gateEntry:
		return d.advanceCycle(inbound, gateEntry, gateConfirm, validationValid, vip)

	case gateExit:
		return d.advanceCycle(outbound, gateExit, gateIdle, true, false)

	case gateReject:
		return d.advanceCycle(inbound, gateReject, gateIdle, true, false)

	default:
		return false, nil
	}
}

func (d *DoubleBarrier) advanceCycle(
	barrier *devices.Barrier,
	current gateStage,
	next gateStage,
	validationValid bool,
	vip bool,
) (bool, error) {
	plcStatus, err := barrier.Status()
	if err != nil {
		return false, err
	}

	other := d.entry
	if other == barrier {
		other = d.exit
	}

	otherStatus, err := other.Status()
	if err != nil {
		return false, err
	}

	phase := phaseFromState(plcStatus.State)

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stage != current {
		return false, nil
	}

	if plcStatus.HasAlarm() || phase == lanestatus.PhaseError ||
		otherStatus.HasAlarm() || otherStatus.State == plc.StateAlarm {
		d.enterAlarmLocked()
		return false, nil
	}

	if phase == lanestatus.PhaseNotReady ||
		otherStatus.State == plc.StateInit {
		return false, nil
	}

	if otherStatus.State != plc.StateClosed {
		log.Printf(
			"[gate] PLC не готов во время проезда: active=%s other=%s",
			plcStatus.State,
			otherStatus.State,
		)

		d.enterAlarmLocked()
		return false, nil
	}

	if plcStatus.State == plc.StateClosed &&
		plcStatus.HasAlarmFlag(plc.AlarmNoEvent) {

		if current == gateExit {
			log.Printf("[gate] NoEvent: проезд через второй шлагбаум не обнаружен")
			d.enterAlarmLocked()
			d.startedAt = time.Time{}
		}

		return false, nil
	}

	if phase != lanestatus.PhaseIdle {
		d.startedAt = time.Time{}
		return false, nil
	}

	if d.startedAt.IsZero() {
		if current == gateEntry {
			if !validationValid {
				log.Printf("[gate] validation expired or invalid: Alarm")
				d.enterAlarmLocked()
				return false, nil
			}

			if vip {
				if err := d.outbound().Start(); err != nil {
					d.enterAlarmLocked()
					return false, err
				}

				d.stage = gateExit
				d.startedAt = time.Now()
				return false, nil
			}
		}

		d.stage = next
		return next == gateIdle, nil
	}

	if time.Since(d.startedAt) > doubleStartTimeout {
		log.Printf("[gate] PLC не начал операцию за %s", doubleStartTimeout)
		d.enterAlarmLocked()
		d.startedAt = time.Time{}

		return false, nil
	}

	return false, nil
}

func (d *DoubleBarrier) Start() error {
	return d.entry.Start()
}

func (d *DoubleBarrier) StartReverse() error {
	return d.entry.StartReverse()
}

func (d *DoubleBarrier) Stop() error {
	entryErr := d.entry.Stop()
	exitErr := d.exit.Stop()

	return errors.Join(entryErr, exitErr)
}

func (d *DoubleBarrier) Open() error {
	return d.entry.Open()
}

func (d *DoubleBarrier) Close() error {
	return d.entry.Close()
}

func (d *DoubleBarrier) Reset() ([]string, error) {
	entryResult := make(chan error, 1)
	exitResult := make(chan error, 1)

	go func() {
		entryResult <- d.entry.Reset()
	}()

	go func() {
		exitResult <- d.exit.Reset()
	}()

	entryErr := <-entryResult
	exitErr := <-exitResult

	resetIDs := make([]string, 0, 2)

	if entryErr == nil || errors.Is(entryErr, plcclient.ErrWriteOutcomeUnknown) {
		resetIDs = append(resetIDs, d.entry.ID)
	}

	if exitErr == nil || errors.Is(exitErr, plcclient.ErrWriteOutcomeUnknown) {
		resetIDs = append(resetIDs, d.exit.ID)
	}

	if err := errors.Join(entryErr, exitErr); err != nil {
		return resetIDs, err
	}

	d.mu.Lock()
	d.stage = gateIdle
	d.alarmPrevious = gateIdle
	d.startedAt = time.Time{}
	d.currentDirection = ""
	d.mu.Unlock()

	return resetIDs, nil
}

func (d *DoubleBarrier) Cancel() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch d.stage {
	case gateEntry, gateEntryHold, gateReject:
		if err := d.inbound().Stop(); err != nil {
			return err
		}

	case gateExit:
		if err := d.outbound().Stop(); err != nil {
			return err
		}

	case gateConfirm:

	default:
		return errors.New("нет активного проезда для отмены")
	}

	d.stage = gateIdle
	d.alarmPrevious = gateIdle
	d.startedAt = time.Time{}
	d.currentDirection = ""

	return nil
}

func (d *DoubleBarrier) RecoverNotReady() ([]string, error) {
	resetIDs := make([]string, 0, 2)

	for _, barrier := range []*devices.Barrier{d.entry, d.exit} {
		status, err := barrier.Status()
		if err != nil {
			return resetIDs, err
		}

		switch {
		case status.State == plc.StateAlarm || status.HasAlarm():
			err = barrier.Reset()

			if err == nil || errors.Is(err, plcclient.ErrWriteOutcomeUnknown) {
				resetIDs = append(resetIDs, barrier.ID)
			}

		case status.State != plc.StateClosed && status.State != plc.StateInit:
			err = barrier.Stop()
		}

		if err != nil {
			return resetIDs, err
		}
	}

	d.mu.Lock()
	d.stage = gateIdle
	d.alarmPrevious = gateIdle
	d.startedAt = time.Time{}
	d.currentDirection = ""
	d.mu.Unlock()

	return resetIDs, nil
}

func (d *DoubleBarrier) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var active, other *devices.Barrier

	switch d.stage {
	case gateEntry, gateEntryHold, gateReject:
		active, other = d.inbound(), d.outbound()

	case gateExit:
		active, other = d.outbound(), d.inbound()

	default:
		return nil
	}

	activeStatus, err := active.Status()
	if err != nil {
		return err
	}

	otherStatus, err := other.Status()
	if err != nil {
		return err
	}

	if activeStatus.State == plc.StateAlarm || activeStatus.HasAlarm() ||
		otherStatus.State == plc.StateAlarm || otherStatus.HasAlarm() {
		return errors.New("возврат из Stopping запрещён: PLC сообщает Alarm")
	}

	if otherStatus.State != plc.StateClosed {
		return fmt.Errorf(
			"возврат из Stopping запрещён: второй шлагбаум %s",
			otherStatus.State,
		)
	}

	switch activeStatus.State {
	case plc.StateClosed,
		plc.StateOpening,
		plc.StateOpened,
		plc.StateClosing:
	default:
		return fmt.Errorf(
			"возврат из Stopping запрещён: активный шлагбаум %s",
			activeStatus.State,
		)
	}

	if activeStatus.State == plc.StateClosed && !d.startedAt.IsZero() {
		d.startedAt = time.Now()
	}

	if d.stage == gateEntryHold {
		d.stage = gateEntry
	}

	return nil
}

func (d *DoubleBarrier) StopInit(barrierID string) error {
	switch barrierID {
	case d.entry.ID:
		return d.entry.Stop()
	case d.exit.ID:
		return d.exit.Stop()
	default:
		return errors.New("шлагбаум не принадлежит линии")
	}
}

func (d *DoubleBarrier) waitReadyForStart() error {
	entryResult := make(chan error, 1)
	exitResult := make(chan error, 1)

	go func() {
		entryResult <- d.entry.WaitReadyForStart()
	}()

	go func() {
		exitResult <- d.exit.WaitReadyForStart()
	}()

	return errors.Join(
		<-entryResult,
		<-exitResult,
	)
}

func (d *DoubleBarrier) Snapshot() (lanestatus.Snapshot, error) {
	entryStatus, err := d.entry.Status()
	if err != nil {
		return lanestatus.Snapshot{}, err
	}

	exitStatus, err := d.exit.Status()
	if err != nil {
		return lanestatus.Snapshot{}, err
	}

	d.mu.Lock()
	stage := d.stage
	d.mu.Unlock()

	entryPhase := phaseFromState(entryStatus.State)
	exitPhase := phaseFromState(exitStatus.State)

	phase := doubleBarrierPhase(
		entryPhase,
		exitPhase,
	)

	alarm := stage == gateAlarm ||
		entryStatus.HasAlarm() ||
		exitStatus.HasAlarm() ||
		entryStatus.State == plc.StateAlarm ||
		exitStatus.State == plc.StateAlarm

	if alarm {
		phase = lanestatus.PhaseError
	}

	return lanestatus.Snapshot{
		Phase: phase,
		Alarm: alarm,
		Ready: !alarm &&
			entryStatus.State == plc.StateClosed &&
			exitStatus.State == plc.StateClosed,
		Stage: gateStageName(stage),
		Devices: map[string]lanestatus.DeviceStatus{
			d.entry.ID: {
				Type:       "barrier",
				State:      entryStatus.State.String(),
				Controller: d.entry.ControllerID,
			},
			d.exit.ID: {
				Type:       "barrier",
				State:      exitStatus.State.String(),
				Controller: d.exit.ControllerID,
			},
		},
	}, nil
}

func (d *DoubleBarrier) NextState(validationValid bool, vip bool) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch d.stage {
	case gateEntry, gateEntryHold:
		if !validationValid {
			d.enterAlarmLocked()
			d.startedAt = time.Time{}
			return false, nil
		}

		if err := d.waitReadyForStart(); err != nil {
			return false, err
		}

		if vip {
			if err := d.outbound().Start(); err != nil {
				d.enterAlarmLocked()
				return false, err
			}

			d.stage = gateExit
			d.startedAt = time.Now()
			return false, nil
		}

		d.stage = gateConfirm
		d.startedAt = time.Time{}
		return false, nil

	case gateConfirm:
		if err := d.waitReadyForStart(); err != nil {
			return false, err
		}

		outbound := d.outbound()

		if err := outbound.Start(); err != nil {
			return false, err
		}

		d.stage = gateExit
		d.startedAt = time.Now()

		return false, nil

	case gateExit, gateReject:
		d.stage = gateIdle
		d.startedAt = time.Time{}

		return true, nil

	default:
		return false, errors.New(
			"next_state недоступен в текущем состоянии",
		)
	}
}

func (d *DoubleBarrier) PrevState(validationValid bool, vip bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entryStatus, err := d.entry.Status()
	if err != nil {
		return err
	}

	exitStatus, err := d.exit.Status()
	if err != nil {
		return err
	}

	if entryStatus.State == plc.StateAlarm || entryStatus.HasAlarm() ||
		exitStatus.State == plc.StateAlarm || exitStatus.HasAlarm() {
		d.enterAlarmLocked()
		return errors.New("prev_state запрещён: PLC сообщает Alarm; сначала устранить аварию")
	}

	switch d.stage {
	case gateEntry, gateEntryHold:
		return nil

	case gateConfirm:
		if err := d.waitReadyForStart(); err != nil {
			return err
		}

		if err := d.inbound().Start(); err != nil {
			d.enterAlarmLocked()
			return err
		}

		d.stage = gateEntry
		d.startedAt = time.Now()
		return nil

	case gateExit:
		if !validationValid || vip {
			return nil
		}

		if err := d.outbound().Stop(); err != nil {
			d.enterAlarmLocked()
			return err
		}

		if err := d.waitReadyForStart(); err != nil {
			d.enterAlarmLocked()
			return err
		}

		if err := d.inbound().Start(); err != nil {
			d.enterAlarmLocked()
			return err
		}

		d.stage = gateEntry
		d.startedAt = time.Now()
		return nil

	case gateAlarm:
		previous := d.alarmPrevious

		if previous == gateIdle {
			return errors.New("prev_state недоступен: предыдущее состояние неизвестно")
		}

		if previous == gateEntry && !validationValid {
			return errors.New("prev_state недоступен: валидация просрочена или недействительна")
		}

		if err := d.waitReadyForStart(); err != nil {
			return err
		}

		if previous == gateConfirm {
			d.stage = gateConfirm
			d.alarmPrevious = gateIdle
			d.startedAt = time.Time{}
			return nil
		}

		barrier := d.inbound()

		if previous == gateExit {
			barrier = d.outbound()
		}

		if previous != gateEntry &&
			previous != gateEntryHold &&
			previous != gateExit &&
			previous != gateReject {
			return errors.New("prev_state недоступен: неизвестное состояние до Alarm")
		}

		if err := barrier.Start(); err != nil {
			return err
		}

		if previous == gateEntryHold {
			previous = gateEntry
		}

		d.stage = previous
		d.alarmPrevious = gateIdle
		d.startedAt = time.Now()

		return nil

	default:
		return errors.New("prev_state недоступен в текущем состоянии")
	}
}

func doubleBarrierPhase(
	entryPhase lanestatus.Phase,
	exitPhase lanestatus.Phase,
) lanestatus.Phase {
	if entryPhase == lanestatus.PhaseError ||
		exitPhase == lanestatus.PhaseError {
		return lanestatus.PhaseError
	}

	if entryPhase == lanestatus.PhaseNotReady ||
		exitPhase == lanestatus.PhaseNotReady {
		return lanestatus.PhaseNotReady
	}

	if entryPhase == lanestatus.PhaseOpening ||
		exitPhase == lanestatus.PhaseOpening {
		return lanestatus.PhaseOpening
	}

	if entryPhase == lanestatus.PhaseClosing ||
		exitPhase == lanestatus.PhaseClosing {
		return lanestatus.PhaseClosing
	}

	if entryPhase == lanestatus.PhaseOpened ||
		exitPhase == lanestatus.PhaseOpened {
		return lanestatus.PhaseOpened
	}

	return lanestatus.PhaseIdle
}

func gateStageName(stage gateStage) string {
	switch stage {
	case gateEntry, gateEntryHold:
		return "entry"
	case gateConfirm:
		return "confirm"
	case gateExit:
		return "exit"
	case gateReject:
		return "reject"
	case gateAlarm:
		return "alarm"
	default:
		return "idle"
	}
}
