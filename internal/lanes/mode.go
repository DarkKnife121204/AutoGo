package lanes

type Mode string

const (
	ModeAutomatic Mode = "automatic"
	ModeManual    Mode = "manual"
)

func ParseMode(raw string) Mode {
	switch raw {
	case string(ModeManual):
		return ModeManual
	default:
		return ModeAutomatic
	}
}
