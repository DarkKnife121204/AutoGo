package access

import (
	"context"
	"strings"
)

type StubDecider struct{}

func NewStubDecider() *StubDecider {
	return &StubDecider{}
}

func (d *StubDecider) Decide(
	_ context.Context,
	req Request,
) (Decision, error) {
	value := req.Plate
	if value == "" {
		value = req.KeyCode
	}

	if strings.EqualFold(strings.TrimSpace(value), "DENY") {
		return Decision{
			Allowed: false,
			Reason:  "отказано заглушкой",
		}, nil
	}

	return Decision{
		Allowed: true,
		Reason:  "разрешено заглушкой",
	}, nil
}
