package plc

type ActualState int32

const (
	ActualStateUnknown      ActualState = 0
	ActualStateClosed       ActualState = 1
	ActualStateOpened       ActualState = 3
	ActualStateIntermediate ActualState = 5
	ActualStateInvalid      ActualState = 6
)

func (s ActualState) String() string {
	switch s {
	case ActualStateClosed:
		return "Closed"
	case ActualStateOpened:
		return "Opened"
	case ActualStateIntermediate:
		return "Intermediate"
	case ActualStateInvalid:
		return "Invalid"
	default:
		return "Unknown"
	}
}
