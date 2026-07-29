package plc

import "math"

func EncodeInt32(value int32) [2]uint16 {
	raw := uint32(value)

	return [2]uint16{
		uint16(raw >> 16),
		uint16(raw),
	}
}

func DecodeInt32(high, low uint16) int32 {
	raw := uint32(high)<<16 | uint32(low)

	return int32(raw)
}

func EncodeFloat32(value float32) [2]uint16 {
	raw := math.Float32bits(value)

	return [2]uint16{
		uint16(raw >> 16),
		uint16(raw),
	}
}

func DecodeFloat32(high, low uint16) float32 {
	raw := uint32(high)<<16 | uint32(low)

	return math.Float32frombits(raw)
}
