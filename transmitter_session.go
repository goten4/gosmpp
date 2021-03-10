package gosmpp

import (
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

// TransmitterSession represents session for Transmitter.
type TransmitterSession struct {
	*session
}

// TransmitSettings is configuration for Transmitter.
type TransmitSettings clientSettings

// NewTransmitterSession creates new session for Transmitter.
//
// Session will `non-stop`, automatically rebind (create new and authenticate connection with SMSC) when
// unexpected error happened.
//
// `rebindingInterval` indicates duration that Session has to wait before rebinding again.
//
// Setting `rebindingInterval <= 0` will disable `auto-rebind` functionality.
func NewTransmitterSession(dialer Dialer, auth Auth, settings TransmitSettings, rebindingInterval time.Duration) (*TransmitterSession, error) {
	session, err := newSession(pdu.Transmitter, dialer, auth, clientSettings(settings), rebindingInterval)
	return &TransmitterSession{session}, err
}

// Transmitter returns bound Transmitter.
func (s *TransmitterSession) Transmitter() Transmitter {
	return Transmitter(s.Client())
}
