package plc

type State int32

const (
	StateUnknown         State = 0
	StateWaiting         State = 1
	StateWaitingTransfer State = 2
	StateOpeningBarrier  State = 3
	StateClosingBarrier  State = 4
	StateAlarm           State = 5
	StateInit            State = 6
)

func (s State) String() string {
	switch s {
	case StateWaiting:
		return "Waiting"
	case StateWaitingTransfer:
		return "WaitingTransfer"
	case StateOpeningBarrier:
		return "OpeningBarrier"
	case StateClosingBarrier:
		return "ClosingBarrier"
	case StateAlarm:
		return "Alarm"
	case StateInit:
		return "Init"
	default:
		return "Unknown"
	}
}
