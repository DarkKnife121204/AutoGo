package scenarios

import (
	"errors"
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
	d.currentDirection = direction
	d.stage = gateEntry
	d.startedAt = time.Now()
	inbound := d.inbound()
	d.mu.Unlock()

	return d.startBarrier(inbound)
}

func (d *DoubleBarrier) Confirm() error {
	d.mu.Lock()

	if d.stage != gateConfirm {
		d.mu.Unlock()

		return errors.New(
			"подтверждение недоступно: машина не в шлюзе",
		)
	}

	d.stage = gateExit
	d.startedAt = time.Now()
	outbound := d.outbound()
	d.mu.Unlock()

	return d.startBarrier(outbound)
}

func (d *DoubleBarrier) Reject() error {
	d.mu.Lock()

	if d.stage != gateConfirm {
		d.mu.Unlock()

		return errors.New(
			"отклонение недоступно: машина не в шлюзе",
		)
	}

	d.stage = gateReject
	d.startedAt = time.Now()
	inbound := d.inbound()
	d.mu.Unlock()

	return d.startBarrier(inbound)
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
	if err := d.entry.Stop(); err != nil {
		return err
	}

	return d.exit.Stop()
}

func (d *DoubleBarrier) Open() error {
	return d.entry.Open()
}

func (d *DoubleBarrier) Close() error {
	return d.entry.Close()
}

func (d *DoubleBarrier) Reset() error {
	if err := d.entry.Reset(); err != nil {
		return err
	}

	return d.exit.Reset()
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

	phase := lanestatus.PhaseIdle
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
