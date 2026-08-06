package lanes

import (
	"errors"
	"fmt"

	"AutoGo/internal/plcclient"
	"AutoGo/internal/scenarios"
)

type Lane struct {
	ID           string
	Name         string
	CheckpointID string
	Scenario     scenarios.Scenario
}

func New(
	id string,
	name string,
	checkpointID string,
	scenario scenarios.Scenario,
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
	}, nil
}

func (l *Lane) ScenarioType() string {
	return l.Scenario.Type()
}

func (l *Lane) Start() error {
	return l.Scenario.Start()
}

func (l *Lane) StartReverse() error {
	return l.Scenario.StartReverse()
}

func (l *Lane) Stop() error {
	return l.Scenario.Stop()
}

func (l *Lane) Open() error {
	return l.Scenario.Open()
}

func (l *Lane) Close() error {
	return l.Scenario.Close()
}

func (l *Lane) Reset() error {
	return l.Scenario.Reset()
}

func (l *Lane) Status() (
	plcclient.Status,
	error,
) {
	return l.Scenario.Status()
}
