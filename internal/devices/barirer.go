package devices

import (
	"fmt"

	"AutoGo/internal/plcclient"
)

type Barrier struct {
	ID           string
	Name         string
	ControllerID string
	PLC          *plcclient.Client
}

func NewBarrier(
	id string,
	name string,
	controllerID string,
	plc *plcclient.Client,
) (*Barrier, error) {
	if plc == nil {
		return nil, fmt.Errorf(
			"для шлагбаума %q не указан PLC-клиент",
			id,
		)
	}

	return &Barrier{
		ID:           id,
		Name:         name,
		ControllerID: controllerID,
		PLC:          plc,
	}, nil
}

func (b *Barrier) Start() error {
	return b.PLC.Start()
}

func (b *Barrier) StartReverse() error {
	return b.PLC.StartReverse()
}

func (b *Barrier) Stop() error {
	return b.PLC.Stop()
}

func (b *Barrier) Open() error {
	return b.PLC.Open()
}

func (b *Barrier) Close() error {
	return b.PLC.CloseBarrier()
}

func (b *Barrier) Reset() error {
	return b.PLC.Reset()
}

func (b *Barrier) Status() (plcclient.Status, error) {
	return b.PLC.Status()
}
