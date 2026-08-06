package plc

type State int32

const (
	StateInit    State = 0
	StateClosed  State = 1
	StateOpening State = 2
	StateOpened  State = 3
	StateClosing State = 4
	StateStopped State = 5
	StateAlarm   State = 6
)

func (s State) String() string {
	switch s {
	case StateInit:
		return "Init"
	case StateClosed:
		return "Closed"
	case StateOpening:
		return "Opening"
	case StateOpened:
		return "Opened"
	case StateClosing:
		return "Closing"
	case StateStopped:
		return "Stopped"
	case StateAlarm:
		return "Alarm"
	default:
		return "Unknown"
	}
}
