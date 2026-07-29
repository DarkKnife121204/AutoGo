package plc

type Command int32

const (
	CommandNone         Command = 0
	CommandStop         Command = 1
	CommandStart        Command = 2
	CommandStartReverse Command = 3
	CommandOpen         Command = 4
	CommandClose        Command = 5
	CommandReset        Command = 32768
)

func (c Command) String() string {
	switch c {
	case CommandStop:
		return "Stop"
	case CommandStart:
		return "Start"
	case CommandStartReverse:
		return "StartReverse"
	case CommandOpen:
		return "Open"
	case CommandClose:
		return "Close"
	case CommandReset:
		return "Reset"
	default:
		return "None"
	}
}
