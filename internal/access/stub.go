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
	if strings.EqualFold(strings.TrimSpace(req.Credential), "DENY") {
		return Decision{
			Allowed: false,
			Reason:  "отказано заглушкой (credential=DENY)",
		}, nil
	}

	return Decision{
		Allowed: true,
		Reason:  "разрешено заглушкой",
	}, nil
}
