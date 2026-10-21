package access

import "context"

type DisabledDecider struct{}

func (d *DisabledDecider) Decide(_ context.Context, _ Request) (Decision, error) {
	return Decision{
		Allowed:   false,
		PlateType: PlateTypeInvalid,
		Reason:    "сервис проверки доступа отключён",
	}, nil
}
