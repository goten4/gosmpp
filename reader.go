package gosmpp

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

const (
	defaultReadTimeout = 2 * time.Second
)

// readerSettings is event listener for reader.
type readerSettings struct {
	// timeout represents conn read timeout.
	// This field is very important to detect connection failure.
	// Default: 2 secs
	timeout time.Duration

	// onPDU handles received PDU from SMSC.
	//
	// `Responded` flag indicates this pdu is responded automatically,
	// no manual respond needed.
	onPDU PDUCallback

	// onReceivingError notifies happened error while reading PDU
	// from SMSC.
	onReceivingError ErrorCallback

	// onRebindingError notifies error while rebinding.
	onRebindingError ErrorCallback

	// onClosed notifies `closed` event due to State.
	onClosed ClosedCallback

	response func(pdu.PDU)
}

func (s *readerSettings) normalize() {
	if s.timeout <= 0 {
		s.timeout = defaultReadTimeout
	}
}

type reader struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	settings readerSettings
	conn     *Connection
	state    int32
}

func newReader(conn *Connection, settings readerSettings) (r *reader) {
	settings.normalize()

	r = &reader{
		settings: settings,
		conn:     conn,
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	return
}

// SystemID returns tagged SystemID, returned from bind_resp from SMSC.
func (t *reader) SystemID() string {
	return t.conn.systemID
}

// Close reader, close connection and stop underlying daemons.
func (t *reader) Close() (err error) {
	return t.close(ExplicitClosing)
}

func (t *reader) close(state State) (err error) {
	if atomic.CompareAndSwapInt32(&t.state, 0, 1) {
		// cancel to notify stop
		t.cancel()

		// set read deadline for current blocking read
		_ = t.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))

		// wait daemons
		t.wg.Wait()

		// close connection to notify daemons to stop
		if state != StoppingProcessOnly {
			err = t.conn.Close()
		}

		// notify reader closed
		if t.settings.onClosed != nil {
			t.settings.onClosed(state)
		}
	}
	return
}

func (t *reader) closing(state State) {
	go func() {
		_ = t.close(state)
	}()
}

func (t *reader) start() {
	t.wg.Add(1)
	go func() {
		t.loop()
		t.wg.Done()
	}()
}

// check error and do closing if need
func (t *reader) check(err error) (closing bool) {
	if err == nil {
		return
	}

	if t.settings.onReceivingError != nil {
		t.settings.onReceivingError(err)
	}

	closing = true
	return
}

// PDU loop processing
func (t *reader) loop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		// read pdu from conn
		var p pdu.PDU
		err := t.conn.SetReadTimeout(t.settings.timeout)
		if err == nil {
			p, err = pdu.Parse(t.conn)
		}

		// check error
		if closeOnError := t.check(err); closeOnError || t.handleOrClose(p) {
			if closeOnError {
				t.closing(InvalidStreaming)
			}
			return
		}
	}
}

func (t *reader) handleOrClose(p pdu.PDU) (closing bool) {
	if p != nil {
		switch pp := p.(type) {
		case *pdu.EnquireLink:
			if t.settings.response != nil {
				t.settings.response(pp.GetResponse())
			}

		case *pdu.Unbind:
			if t.settings.response != nil {
				t.settings.response(pp.GetResponse())

				// wait to send response before closing
				time.Sleep(50 * time.Millisecond)
			}

			closing = true
			t.closing(UnbindClosing)

		default:
			var responded bool
			if p.CanResponse() && t.settings.response != nil {
				t.settings.response(p.GetResponse())
				responded = true
			}

			if t.settings.onPDU != nil {
				t.settings.onPDU(p, responded)
			}
		}
	}
	return
}
