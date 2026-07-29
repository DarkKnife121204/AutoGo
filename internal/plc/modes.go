package plc

type Mode int32

const (
	ModeUnknown   Mode = 0
	ModeAutomatic Mode = 1
	ModeManual    Mode = 2
)

func (m Mode) String() string {
	switch m {
	case ModeAutomatic:
		return "Automatic"
	case ModeManual:
		return "Manual"
	default:
		return "Unknown"
	}
}
