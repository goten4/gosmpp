package gosmpp

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linxGnu/gosmpp/data"
	"github.com/linxGnu/gosmpp/pdu"
)

func TestReceiverSession(t *testing.T) {
	auth := nextAuth()
	receiver, err := NewSession(pdu.Receiver, NonTLSDialer, auth, ClientSettings{
		OnReceivingError: func(err error) {
			fmt.Println(err)
		},
		OnRebindingError: func(err error) {
			fmt.Println(err)
		},
		OnPDU: func(p pdu.PDU, responded bool) {
			fmt.Println(p)
		},
		OnClosed: func(state State) {
			fmt.Println(state)
		},
	}, 5*time.Second)
	require.Nil(t, err)
	require.NotNil(t, receiver)
	defer func() {
		_ = receiver.Close()
	}()

	require.Equal(t, "MelroseLabsSMSC", receiver.Client().SystemID())

	time.Sleep(time.Second)
	receiver.rebind()
}

func TestTransmitterSession(t *testing.T) {
	t.Run("binding", func(t *testing.T) {
		auth := nextAuth()
		session, err := NewSession(pdu.Transmitter, NonTLSDialer, auth, ClientSettings{
			WriteTimeout: time.Second,

			OnSubmitError: func(p pdu.PDU, err error) {
				t.Fatal(err)
			},
			OnRebindingError: func(err error) {
				t.Fatal(err)
			},
			OnClosed: func(state State) {
				fmt.Println(state)
			},
		}, 5*time.Second)
		require.Nil(t, err)
		require.NotNil(t, session)
		defer func() {
			_ = session.Close()
		}()

		require.Equal(t, "MelroseLabsSMSC", session.Client().SystemID())

		err = session.Client().Submit(newSubmitSM(auth.SystemID))
		require.Nil(t, err)

		time.Sleep(400 * time.Millisecond)

		session.rebind()
		err = session.Client().Submit(newSubmitSM(auth.SystemID))
		require.Nil(t, err)
	})

	errorHandling := func(t *testing.T, trigger func(*writer)) {
		conn, err := net.Dial("tcp", "smscsim.melroselabs.com:2775")
		require.Nil(t, err)

		var tr writer
		tr.input = make(chan pdu.PDU, 1)

		c := NewConnection(conn)
		defer func() {
			_ = c.Close()

			// write on closed conn?
			n, err := tr.write([]byte{1, 2, 3})
			require.NotNil(t, err)
			require.Zero(t, n)
		}()

		// fake settings
		tr.conn = c
		tr.ctx, tr.cancel = context.WithCancel(context.Background())

		var count int32
		tr.settings.onClosed = func(state State) {
			atomic.AddInt32(&count, 1)
		}

		tr.settings.onSubmitError = func(p pdu.PDU, err error) {
			require.NotNil(t, err)
			_, ok := p.(*pdu.CancelSM)
			require.True(t, ok)
		}

		// do trigger
		trigger(&tr)

		time.Sleep(300 * time.Millisecond)
		require.NotZero(t, atomic.LoadInt32(&count))
	}

	t.Run("errorHandling1", func(t *testing.T) {
		errorHandling(t, func(tr *writer) {
			var p pdu.CancelSM
			tr.check(&p, 100, fmt.Errorf("fake error"))
		})
	})

	t.Run("errorHandling2", func(t *testing.T) {
		errorHandling(t, func(tr *writer) {
			var p pdu.CancelSM
			tr.check(&p, 0, fmt.Errorf("fake error"))
		})
	})

	t.Run("errorHandling3", func(t *testing.T) {
		errorHandling(t, func(tr *writer) {
			var p pdu.CancelSM
			tr.check(&p, 0, &net.DNSError{IsTemporary: false})
		})
	})
}

var (
	countSubmitSMResp, countDeliverSM int32
)

func TestTransceiverSession(t *testing.T) {
	auth := nextAuth()
	trans, err := NewSession(pdu.Transceiver, NonTLSDialer, auth, ClientSettings{
		EnquireLink: 200 * time.Millisecond,

		OnSubmitError: func(p pdu.PDU, err error) {
			t.Fatal(err)
		},

		OnReceivingError: func(err error) {
			fmt.Println(err)
		},

		OnRebindingError: func(err error) {
			fmt.Println(err)
		},

		OnPDU: handlePDU(t),

		OnClosed: func(state State) {
			fmt.Println(state)
		},
	}, 5*time.Second)
	require.Nil(t, err)
	require.NotNil(t, trans)
	defer func() {
		_ = trans.Close()
	}()

	require.Equal(t, "MelroseLabsSMSC", trans.Client().SystemID())

	// sending 20 SMS
	for i := 0; i < 20; i++ {
		err = trans.Client().Submit(newSubmitSM(auth.SystemID))
		require.Nil(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(5 * time.Second)

	// wait response received
	require.EqualValues(t, 20, atomic.LoadInt32(&countSubmitSMResp))

	// rebind and submit again
	trans.rebind()
	err = trans.Client().Submit(newSubmitSM(auth.SystemID))
	require.Nil(t, err)
	time.Sleep(time.Second)
	require.EqualValues(t, 21, atomic.LoadInt32(&countSubmitSMResp))
}

func handlePDU(t *testing.T) func(pdu.PDU, bool) {
	return func(p pdu.PDU, responded bool) {
		switch pd := p.(type) {
		case *pdu.SubmitSMResp:
			require.False(t, responded)
			require.EqualValues(t, data.ESME_ROK, pd.CommandStatus)
			require.NotZero(t, len(pd.MessageID))
			atomic.AddInt32(&countSubmitSMResp, 1)

		case *pdu.GenericNack:
			require.False(t, responded)
			t.Fatal(pd)

		case *pdu.DataSM:
			require.True(t, responded)
			fmt.Println(pd.Header)

		case *pdu.DeliverSM:
			require.True(t, responded)
			require.EqualValues(t, data.ESME_ROK, pd.CommandStatus)

			_mess, err := pd.Message.GetMessageWithEncoding(data.UCS2)
			assert.Nil(t, err)
			if mess == _mess {
				atomic.AddInt32(&countDeliverSM, 1)
			}
		}
	}
}

func newSubmitSM(systemID string) *pdu.SubmitSM {
	// build up submitSM
	srcAddr := pdu.NewAddress()
	srcAddr.SetTon(5)
	srcAddr.SetNpi(0)
	_ = srcAddr.SetAddress(systemID)

	destAddr := pdu.NewAddress()
	destAddr.SetTon(1)
	destAddr.SetNpi(1)
	_ = destAddr.SetAddress("12" + systemID)

	submitSM := pdu.NewSubmitSM().(*pdu.SubmitSM)
	submitSM.SourceAddr = srcAddr
	submitSM.DestAddr = destAddr
	_ = submitSM.Message.SetMessageWithEncoding(mess, data.UCS2)
	submitSM.ProtocolID = 0
	submitSM.RegisteredDelivery = 1
	submitSM.ReplaceIfPresentFlag = 0
	submitSM.EsmClass = 0

	return submitSM
}
