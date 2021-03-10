package gosmpp

import (
	"sync/atomic"
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

// session represents SMPP session.
type session struct {
	dialer Dialer
	auth   Auth

	originalOnClosed func(State)
	settings         clientSettings

	bindingType       pdu.BindingType
	rebindingInterval time.Duration

	r atomic.Value // client

	state     int32
	rebinding int32
}

// newSession creates new SMPP session.
//
// Session will `non-stop`, automatically rebind (create new and authenticate connection with SMSC) when
// unexpected error happened.
//
// `rebindingInterval` indicates duration that Session has to wait before rebinding again.
//
// Setting `rebindingInterval <= 0` will disable `auto-rebind` functionality.
func newSession(b pdu.BindingType, dialer Dialer, auth Auth, settings clientSettings, rebindingInterval time.Duration) (s *session, err error) {
	conn, err := connectAs(b, dialer, auth)
	if err == nil {
		s = &session{
			dialer:            dialer,
			auth:              auth,
			bindingType:       b,
			rebindingInterval: rebindingInterval,
			originalOnClosed:  settings.OnClosed,
		}

		if rebindingInterval > 0 {
			newSettings := settings
			newSettings.OnClosed = func(state State) {
				switch state {
				case ExplicitClosing:
					return

				default:
					if s.originalOnClosed != nil {
						s.originalOnClosed(state)
					}
					s.rebind()
				}
			}
			s.settings = newSettings
		} else {
			s.settings = settings
		}

		// create new client
		c := newClient(conn, s.settings)

		// bind to session
		s.r.Store(c)
	}
	return
}

// Client returns bound client.
func (s *session) Client() (c *client) {
	c, _ = s.r.Load().(*client)
	return
}

// Close session.
func (s *session) Close() (err error) {
	if atomic.CompareAndSwapInt32(&s.state, 0, 1) {
		// close underlying client
		err = s.close()
	}
	return
}

// close underlying client
func (s *session) close() (err error) {
	if c := s.Client(); c != nil {
		err = c.Close()
	}
	return
}

func (s *session) rebind() {
	if atomic.CompareAndSwapInt32(&s.rebinding, 0, 1) {
		// close underlying client
		_ = s.close()

		for atomic.LoadInt32(&s.state) == 0 {
			conn, err := connectAs(s.bindingType, s.dialer, s.auth)
			if err != nil {
				if s.settings.OnRebindingError != nil {
					s.settings.OnRebindingError(err)
				}
				time.Sleep(s.rebindingInterval)
			} else {
				c := newClient(conn, s.settings)

				// bind to session
				s.r.Store(c)

				// reset rebinding state
				atomic.StoreInt32(&s.rebinding, 0)

				return
			}
		}
	}
}
