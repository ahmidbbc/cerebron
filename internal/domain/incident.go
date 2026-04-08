package domain

import "time"

type Incident struct {
	ID          string
	Service     string
	Environment string
	Severity    Severity
	Fingerprint string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	OwnerTeam   string
	Status      string
}
