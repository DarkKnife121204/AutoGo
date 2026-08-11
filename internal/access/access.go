package access

import "context"

type Request struct {
	LaneID     string
	Credential string
	Source     string
	Direction  string
}

type Decision struct {
	Allowed bool
	Reason  string
}

type Decider interface {
	Decide(ctx context.Context, req Request) (Decision, error)
}
