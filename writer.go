package gosmpp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

const (
	// EnquireLinkIntervalMinimum represents minimum interval for enquire link.
	EnquireLinkIntervalMinimum = 20 * time.Second
)

var (
	// ErrSessionClosing indicates session is closing, cannot send any PDU.
	ErrSessionClosing = fmt.Errorf("client is closing, cannot send PDU to SMSC")
)

// writerSettings is listener for writer.
type writerSettings struct {
	// timeout is timeout/deadline for submitting PDU.
	timeout time.Duration

	// enquireLink periodically sends enquireLink to SMSC.
	// The duration must not be smaller than 1 minute.
	//
	// Zero duration disables auto enquire link.
	enquireLink time.Duration

	// onSubmitError notifies fail-to-submit PDU with along error.
	onSubmitError PDUErrorCallback

	// onRebindingError notifies error while rebinding.
	onRebindingError ErrorCallback

	// onClosed notifies `closed` event due to State.
	onClosed ClosedCallback
}

func (s *writerSettings) normalize() {
	if s.enquireLink <= EnquireLinkIntervalMinimum {
		s.enquireLink = EnquireLinkIntervalMinimum
	}
}

type writer struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	settings writerSettings
	conn     *Connection
	input    chan pdu.PDU
	lock     sync.RWMutex
	state    int32
}

func newWriter(conn *Connection, settings writerSettings) (w *writer) {
	settings.normalize()

	w = &writer{
		settings: settings,
		conn:     conn,
		input:    make(chan pdu.PDU, 1),
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	return
}

// Close writer and stop underlying daemons.
func (t *writer) Close() (err error) {
	return t.close(ExplicitClosing)
}

func (t *writer) close(state State) (err error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.state == 0 {
		// don't receive anymore SubmitSM
		t.cancel()

		// notify daemon
		close(t.input)

		// wait daemon
		t.wg.Wait()

		// try to send unbind
		_, _ = t.conn.Write(marshal(pdu.NewUnbind()))

		// close connection
		if state != StoppingProcessOnly {
			err = t.conn.Close()
		}

		// notify writer closed
		if t.settings.onClosed != nil {
			t.settings.onClosed(state)
		}

		t.state = 1
	}

	return
}

func (t *writer) closing(state State) {
	go func() {
		_ = t.close(state)
	}()
}

// submit a PDU.
func (t *writer) submit(p pdu.PDU) (err error) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if t.state == 0 {
		select {
		case <-t.ctx.Done():
			err = t.ctx.Err()

		case t.input <- p:
		}
	} else {
		err = ErrSessionClosing
	}

	return
}

func (t *writer) start() {
	t.wg.Add(1)
	if t.settings.enquireLink > 0 {
		go func() {
			t.loopWithEnquireLink()
			t.wg.Done()
		}()
	} else {
		go func() {
			t.loop()
			t.wg.Done()
		}()
	}
}

// PDU loop processing
func (t *writer) loop() {
	for p := range t.input {
		if p != nil {
			n, err := t.write(marshal(p))
			if t.check(p, n, err) {
				return
			}
		}
	}
}

// PDU loop processing with enquire link support
func (t *writer) loopWithEnquireLink() {
	if t.settings.enquireLink < EnquireLinkIntervalMinimum {
		t.settings.enquireLink = EnquireLinkIntervalMinimum
	}

	ticker := time.NewTicker(t.settings.enquireLink)
	defer ticker.Stop()

	// enquireLink payload
	eqp := pdu.NewEnquireLink()
	enquireLink := marshal(eqp)

	for {
		select {
		case <-ticker.C:
			n, err := t.write(enquireLink)
			if t.check(eqp, n, err) {
				return
			}

		case p, ok := <-t.input:
			if !ok {
				return
			}

			if p != nil {
				n, err := t.write(marshal(p))
				if t.check(p, n, err) {
					return
				}
			}
		}
	}
}

// check error and do closing if need
func (t *writer) check(p pdu.PDU, n int, err error) (closing bool) {
	if err == nil {
		return
	}

	if t.settings.onSubmitError != nil {
		t.settings.onSubmitError(p, err)
	}

	if n == 0 {
		if nErr, ok := err.(net.Error); ok {
			closing = nErr.Timeout() || !nErr.Temporary()
		} else {
			closing = true
		}
	} else {
		closing = true // force closing
	}

	if closing {
		t.closing(ConnectionIssue) // start closing
	}

	return
}

// low level writing
func (t *writer) write(v []byte) (n int, err error) {
	hasTimeout := t.settings.timeout > 0

	if hasTimeout {
		err = t.conn.SetWriteTimeout(t.settings.timeout)
	}

	if err == nil {
		if n, err = t.conn.Write(v); err != nil &&
			n == 0 &&
			hasTimeout &&
			t.conn.SetWriteTimeout(t.settings.timeout<<1) == nil {
			// retry again with double timeout
			n, err = t.conn.Write(v)
		}
	}

	return
}
