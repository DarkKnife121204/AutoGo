package scenarios

import (
	"errors"
	"log"
	"sync"
	"time"

	"AutoGo/internal/devices"
	"AutoGo/internal/lanestatus"
)

const TypeDoubleBarrier = "double_barrier"

const doubleStartTimeout = 10 * time.Second

type gateStage int

const (
	gateIdle gateStage = iota
	gateEntry
	gateConfirm
	gateExit
	gateReject
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

func (d *DoubleBarrier) startBarrier(b *devices.Barrier) error {
	if d.currentDirection == directionReverse {
		return b.StartReverse()
	}

	return b.Start()
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

	if err := d.startBarrier(inbound); err != nil {
		d.currentDirection = ""

		log.Printf("[gate] Begin startBarrier error: %v", err)

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

	if err := d.startBarrier(outbound); err != nil {
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

	if err := d.startBarrier(inbound); err != nil {
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
		d.stage = gateIdle

		return true, nil
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
		d.stage = gateIdle

		return true, nil
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

	if entryStatus.HasAlarm() || exitStatus.HasAlarm() {
		phase = lanestatus.PhaseError
	}

	return lanestatus.Snapshot{
		Phase: phase,
		Alarm: entryStatus.HasAlarm() || exitStatus.HasAlarm(),
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

func doubleBarrierPhase(
	entryPhase lanestatus.Phase,
	exitPhase lanestatus.Phase,
) lanestatus.Phase {
	if entryPhase == lanestatus.PhaseError ||
		exitPhase == lanestatus.PhaseError {
		return lanestatus.PhaseError
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
	case gateEntry:
		return "entry"
	case gateConfirm:
		return "confirm"
	case gateExit:
		return "exit"
	case gateReject:
		return "reject"
	default:
		return "idle"
	}
}
