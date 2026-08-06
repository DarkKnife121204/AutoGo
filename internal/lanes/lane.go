package lanes

import (
	"errors"
	"fmt"

	"AutoGo/internal/devices"
	"AutoGo/internal/plcclient"
)

const ScenarioSingleBarrier = "single_barrier"

type Lane struct {
	ID           string
	Name         string
	CheckpointID string
	ScenarioType string
	Barrier      *devices.Barrier
}

func NewSingleBarrier(
	id string,
	name string,
	checkpointID string,
	barrier *devices.Barrier,
) (*Lane, error) {
	if id == "" {
		return nil, errors.New("ID линии не указан")
	}

	if barrier == nil {
		return nil, fmt.Errorf(
			"для линии %q не указан шлагбаум",
			id,
		)
	}

	return &Lane{
		ID:           id,
		Name:         name,
		CheckpointID: checkpointID,
		ScenarioType: ScenarioSingleBarrier,
		Barrier:      barrier,
	}, nil
}

func (l *Lane) Start() error {
	return l.Barrier.Start()
}

func (l *Lane) StartReverse() error {
	return l.Barrier.StartReverse()
}

func (l *Lane) Stop() error {
	return l.Barrier.Stop()
}

func (l *Lane) Open() error {
	return l.Barrier.Open()
}

func (l *Lane) Close() error {
	return l.Barrier.Close()
}

func (l *Lane) Reset() error {
	return l.Barrier.Reset()
}

func (l *Lane) Status() (plcclient.Status, error) {
	return l.Barrier.Status()
}
