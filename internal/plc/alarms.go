package plc

type Alarm uint16

const (
	AlarmNone               Alarm = 0
	AlarmBarrierOpen        Alarm = 1
	AlarmBarrierClose       Alarm = 2
	AlarmBarrierIllegalOpen Alarm = 4
	AlarmUnallowedStart     Alarm = 8
	AlarmNoEvent            Alarm = 16
)
