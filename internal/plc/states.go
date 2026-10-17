package plc

type State int32

const (
	StateClosed  State = 1 // UDTGate: Waiting
	StateOpened  State = 2 // UDTGate: WaitingTransfer
	StateOpening State = 3 // UDTGate: OpeningBarrier
	StateClosing State = 4 // UDTGate: ClosingBarrier
	StateAlarm   State = 5 // UDTGate: Alarm
	StateInit    State = 6 // UDTGate: Init
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Waiting"
	case StateOpened:
		return "WaitingTransfer"
	case StateOpening:
		return "OpeningBarrier"
	case StateClosing:
		return "ClosingBarrier"
	case StateAlarm:
		return "Alarm"
	case StateInit:
		return "Init"
	default:
		return "Unknown"
	}
}
