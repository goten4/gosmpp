package gosmpp

import (
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

// ReceiverSession represents session for Receiver.
type ReceiverSession struct {
	*session
}

// ReceiveSettings is configuration for Receiver.
type ReceiveSettings clientSettings

// NewReceiverSession creates new session for Receiver.
//
// Session will `non-stop`, automatically rebind (create new and authenticate connection with SMSC) when
// unexpected error happened.
//
// `rebindingInterval` indicates duration that Session has to wait before rebinding again.
//
// Setting `rebindingInterval <= 0` will disable `auto-rebind` functionality.
func NewReceiverSession(dialer Dialer, auth Auth, settings ReceiveSettings, rebindingInterval time.Duration) (*ReceiverSession, error) {
	session, err := newSession(pdu.Transmitter, dialer, auth, clientSettings(settings), rebindingInterval)
	return &ReceiverSession{session}, err
}

// Receiver returns bound Receiver.
func (s *ReceiverSession) Receiver() Receiver {
	return Receiver(s.Client())
}
