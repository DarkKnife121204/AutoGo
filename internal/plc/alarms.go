package plc

type Alarm uint16

const (
	AlarmNone                  Alarm = 0
	AlarmOpenTimeout           Alarm = 1
	AlarmCloseTimeout          Alarm = 2
	AlarmLimitConflict         Alarm = 4
	AlarmObstacleDuringClosing Alarm = 8
	AlarmPassageTimeout        Alarm = 16
)

func (a Alarm) Has(flag Alarm) bool {
	return a&flag != 0
}
