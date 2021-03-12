package gosmpp

import (
	"sync/atomic"
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

// Session represents SMPP Session.
type Session struct {
	dialer Dialer
	auth   Auth

	originalOnClosed func(State)
	settings         ClientSettings

	bindingType       pdu.BindingType
	rebindingInterval time.Duration

	r atomic.Value // client

	state     int32
	rebinding int32
}

// NewSession creates new SMPP Session.
//
// Session will `non-stop`, automatically rebind (create new and authenticate connection with SMSC) when
// unexpected error happened.
//
// `rebindingInterval` indicates duration that Session has to wait before rebinding again.
//
// Setting `rebindingInterval <= 0` will disable `auto-rebind` functionality.
func NewSession(b pdu.BindingType, dialer Dialer, auth Auth, settings ClientSettings, rebindingInterval time.Duration) (s *Session, err error) {
	conn, err := connectAs(b, dialer, auth)
	if err == nil {
		s = &Session{
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
		c := NewClient(conn, s.settings)

		// bind to session
		s.r.Store(c)
	}
	return
}

// Client returns bound client.
func (s *Session) Client() (c *Client) {
	c, _ = s.r.Load().(*Client)
	return
}

// Close Session.
func (s *Session) Close() (err error) {
	if atomic.CompareAndSwapInt32(&s.state, 0, 1) {
		// close underlying client
		err = s.close()
	}
	return
}

// close underlying client
func (s *Session) close() (err error) {
	if c := s.Client(); c != nil {
		err = c.Close()
	}
	return
}

func (s *Session) rebind() {
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
				c := NewClient(conn, s.settings)

				// bind to session
				s.r.Store(c)

				// reset rebinding state
				atomic.StoreInt32(&s.rebinding, 0)

				return
			}
		}
	}
}
