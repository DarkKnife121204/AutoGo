package access

import "context"

type Request struct {
	LaneID    string
	Plate     string
	KeyCode   string
	Source    string
	Direction string
}

type PlateType string

const (
	PlateTypeInvalid PlateType = "Invalid"
	PlateTypeRegular PlateType = "Regular"
	PlateTypeVIP     PlateType = "VIP"
	PlateTypeUnknown PlateType = "Unknown"
)

type Decision struct {
	Allowed   bool
	PlateType PlateType
	Reason    string
}

type Decider interface {
	Decide(ctx context.Context, req Request) (Decision, error)
}
