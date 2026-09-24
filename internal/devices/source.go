package devices

type Source struct {
	ID         string
	Name       string
	Type       string
	ExternalID string
	Service    string
	Direction  string
}

func NewSource(
	id string,
	name string,
	deviceType string,
	externalID string,
	service string,
	direction string,
) *Source {
	if direction == "" {
		direction = "normal"
	}

	return &Source{
		ID:         id,
		Name:       name,
		Type:       deviceType,
		ExternalID: externalID,
		Service:    service,
		Direction:  direction,
	}
}
