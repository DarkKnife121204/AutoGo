package plc

type Alarm uint16

const (
	AlarmNone               Alarm = 0
	AlarmBarrierAlarmOpen   Alarm = 1
	AlarmBarrierAlarmClose  Alarm = 2
	AlarmBarrierIllegalOpen Alarm = 4
	AlarmUnallowedStart     Alarm = 8
	AlarmNoEvent            Alarm = 16
)

func (a Alarm) Has(value Alarm) bool {
	return a == value
}
