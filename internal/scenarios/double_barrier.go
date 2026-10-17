package scenarios

import (
	"errors"
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

	inbound := d.inbound()

	log.Printf("[gate] Reject: inbound=%s", inbound.ID)

	if err := inbound.Start(); err != nil {
		return err
	}

	d.stage = gateReject
	d.startedAt = time.Now()

	return nil
}

func (d *DoubleBarrier) Advance() (bool, error) {
	d.mu.Lock()
	stage := d.stage
	inbound := d.inbound()
	outbound := d.outbound()
	d.mu.Unlock()

	switch stage {
	case gateEntryHold:
		return false, nil

	case gateEntry:
		return d.advanceCycle(inbound, gateEntry, gateConfirm)

	case gateExit:
		return d.advanceCycle(outbound, gateExit, gateIdle)

	case gateReject:
		return d.advanceCycle(inbound, gateReject, gateIdle)

	default:
		return false, nil
	}
}

func (d *DoubleBarrier) advanceCycle(
	barrier *devices.Barrier,
	current gateStage,
	next gateStage,
) (bool, error) {
	plcStatus, err := barrier.Status()
	if err != nil {
		return false, err
	}

	phase := phaseFromState(plcStatus.State)

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stage != current {
		return false, nil
	}

	if plcStatus.HasAlarm() || phase == lanestatus.PhaseError {
		d.stage = gateAlarm
		d.startedAt = time.Time{}

		return false, nil
	}

	if plcStatus.State == plc.StateClosed &&
		plcStatus.HasAlarmFlag(plc.AlarmNoEvent) {

		if current == gateExit {
			log.Printf("[gate] NoEvent: проезд через второй шлагбаум не обнаружен")
			d.stage = gateAlarm
			d.startedAt = time.Time{}
		}

		return false, nil
	}

	if phase == lanestatus.PhaseNotReady {
		return false, nil
	}

	if phase != lanestatus.PhaseIdle {
		d.startedAt = time.Time{}
		return false, nil
	}

	if d.startedAt.IsZero() {
		d.stage = next

		return next == gateIdle, nil
	}

	if time.Since(d.startedAt) > doubleStartTimeout {
		log.Printf("[gate] PLC не начал операцию за %s", doubleStartTimeout)
		d.stage = gateAlarm
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

func (d *DoubleBarrier) Reset() error {
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

	if err := errors.Join(entryErr, exitErr); err != nil {
		return err
	}

	d.mu.Lock()
	d.stage = gateIdle
	d.startedAt = time.Time{}
	d.currentDirection = ""
	d.mu.Unlock()

	return nil
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
	d.startedAt = time.Time{}
	d.currentDirection = ""

	return nil
}

func (d *DoubleBarrier) RecoverNotReady() error {
	for _, barrier := range []*devices.Barrier{d.entry, d.exit} {
		status, err := barrier.Status()
		if err != nil {
			return err
		}

		switch {
		case status.State == plc.StateAlarm:
			err = barrier.Reset()

		case status.State != plc.StateClosed:
			err = barrier.Stop()
		}

		if err != nil {
			return err
		}
	}

	d.mu.Lock()
	d.stage = gateIdle
	d.startedAt = time.Time{}
	d.currentDirection = ""
	d.mu.Unlock()

	return nil
}

func (d *DoubleBarrier) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var err error

	switch d.stage {
	case gateEntry, gateEntryHold, gateReject:
		err = d.inbound().Start()

	case gateExit:
		err = d.outbound().Start()

	default:
		return nil
	}

	if err != nil {
		return err
	}

	if d.stage == gateEntryHold {
		d.stage = gateEntry
	}

	d.startedAt = time.Now()

	return nil
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
		Ready: entryStatus.State == plc.StateClosed &&
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

func (d *DoubleBarrier) NextState() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch d.stage {
	case gateEntry, gateEntryHold:
		d.stage = gateConfirm
		d.startedAt = time.Time{}

		return false, nil

	case gateConfirm:
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

func (d *DoubleBarrier) PrevState() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch d.stage {
	case gateEntry, gateEntryHold:
		return nil

	case gateConfirm:
		if err := d.inbound().Start(); err != nil {
			return err
		}

	case gateExit:
		// Пока нет кэшированного результата валидации
		return nil

	default:
		return errors.New(
			"prev_state недоступен в текущем состоянии",
		)
	}

	d.stage = gateEntry
	d.startedAt = time.Now()

	return nil
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
