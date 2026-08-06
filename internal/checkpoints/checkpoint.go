package checkpoints

import (
	"errors"
	"fmt"

	"AutoGo/internal/lanes"
)

type Checkpoint struct {
	ID    string
	Name  string
	Lanes map[string]*lanes.Lane
}

func New(
	id string,
	name string,
	checkpointLanes map[string]*lanes.Lane,
) (*Checkpoint, error) {
	if id == "" {
		return nil, errors.New("ID КПП не указан")
	}

	if name == "" {
		return nil, fmt.Errorf(
			"для КПП %q не указано название",
			id,
		)
	}

	if len(checkpointLanes) == 0 {
		return nil, fmt.Errorf(
			"у КПП %q нет включённых линий",
			id,
		)
	}

	return &Checkpoint{
		ID:    id,
		Name:  name,
		Lanes: checkpointLanes,
	}, nil
}

func (c *Checkpoint) Lane(
	id string,
) (*lanes.Lane, bool) {
	lane, exists := c.Lanes[id]

	return lane, exists
}
