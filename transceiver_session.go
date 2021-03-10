package gosmpp

import (
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

// TransceiverSession represents session for Transceiver.
type TransceiverSession struct {
	*session
}

// TransceiveSettings is configuration for Transceiver.
type TransceiveSettings clientSettings

// NewTransceiverSession creates new session for Transceiver.
//
// Session will `non-stop`, automatically rebind (create new and authenticate connection with SMSC) when
// unexpected error happened.
//
// `rebindingInterval` indicates duration that Session has to wait before rebinding again.
//
// Setting `rebindingInterval <= 0` will disable `auto-rebind` functionality.
func NewTransceiverSession(dialer Dialer, auth Auth, settings TransceiveSettings, rebindingInterval time.Duration) (*TransceiverSession, error) {
	session, err := newSession(pdu.Transceiver, dialer, auth, clientSettings(settings), rebindingInterval)
	return &TransceiverSession{session}, err
}

// Transceiver returns bound Transceiver.
func (s *TransceiverSession) Transceiver() Transceiver {
	return Transceiver(s.Client())
}
