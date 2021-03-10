package gosmpp

import (
	"sync/atomic"
	"time"

	"github.com/linxGnu/gosmpp/pdu"
)

// clientSettings is configuration for SMPP client.
type clientSettings struct {
	// WriteTimeout is timeout for submitting PDU.
	WriteTimeout time.Duration

	// ReadTimeout is timeout for reading PDU from SMSC.
	// Underlying net.Conn will be stricted with ReadDeadline(now + timeout).
	// This setting is very important to detect connection failure.
	//
	// Default: 2 secs
	ReadTimeout time.Duration

	// EnquireLink periodically sends EnquireLink to SMSC.
	// Zero duration means disable auto enquire link.
	EnquireLink time.Duration

	// OnPDU handles received PDU from SMSC.
	//
	// `Responded` flag indicates this pdu is responded automatically,
	// no manual respond needed.
	OnPDU PDUCallback

	// OnSubmitError notifies fail-to-submit PDU with along error.
	OnSubmitError PDUErrorCallback

	// OnReceivingError notifies happened error while reading PDU
	// from SMSC.
	OnReceivingError ErrorCallback

	// OnRebindingError notifies error while rebinding.
	OnRebindingError ErrorCallback

	// OnClosed notifies `closed` event due to State.
	OnClosed ClosedCallback
}

type client struct {
	settings clientSettings
	conn     *Connection
	reader   *reader
	writer   *writer
	state    int32
}

// newClient creates new client from bound connection.
func newClient(conn *Connection, settings clientSettings) *client {
	c := &client{
		settings: settings,
		conn:     conn,
	}

	c.writer = newWriter(conn, writerSettings{
		timeout: settings.WriteTimeout,

		enquireLink: settings.EnquireLink,

		onSubmitError: settings.OnSubmitError,

		onClosed: func(state State) {
			switch state {
			case ExplicitClosing:
				return

			case ConnectionIssue:
				// also close input
				_ = c.reader.Close()

				if c.settings.OnClosed != nil {
					c.settings.OnClosed(ConnectionIssue)
				}
			}
		},
	})

	c.reader = newReader(conn, readerSettings{
		timeout: settings.ReadTimeout,

		onPDU: settings.OnPDU,

		onReceivingError: settings.OnReceivingError,

		onClosed: func(state State) {
			switch state {
			case ExplicitClosing:
				return

			case InvalidStreaming, UnbindClosing:
				// also close output
				_ = c.writer.Close()

				if c.settings.OnClosed != nil {
					c.settings.OnClosed(state)
				}
			}
		},

		response: func(p pdu.PDU) {
			if c.writer.submit(p) != nil { // only happened when transceiver is closed
				_, _ = c.writer.write(marshal(p))
			}
		},
	})

	c.writer.start()
	c.reader.start()

	return c
}

// SystemID returns tagged SystemID, returned from bind_resp from SMSC.
func (c *client) SystemID() string {
	return c.conn.systemID
}

// Close transceiver and stop underlying daemons.
func (c *client) Close() (err error) {
	if atomic.CompareAndSwapInt32(&c.state, 0, 1) {
		// closing input and output
		_ = c.writer.close(StoppingProcessOnly)
		_ = c.reader.close(StoppingProcessOnly)

		// close underlying conn
		err = c.conn.Close()

		// notify transceiver closed
		if c.settings.OnClosed != nil {
			c.settings.OnClosed(ExplicitClosing)
		}
	}
	return
}

// Submit a PDU.
func (c *client) Submit(p pdu.PDU) error {
	return c.writer.submit(p)
}
