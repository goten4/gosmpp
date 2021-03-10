package gosmpp

import (
	"net"

	"github.com/linxGnu/gosmpp/pdu"
)

// Dialer is connection dialer.
type Dialer func(addr string) (net.Conn, error)

var (
	// NonTLSDialer is non-tls connection dialer.
	NonTLSDialer = func(addr string) (net.Conn, error) {
		return net.Dial("tcp", addr)
	}
)

// Auth represents basic authentication to SMSC.
type Auth struct {
	// SMSC represents SMSC address.
	SMSC string

	// authentication infos
	SystemID   string
	Password   string
	SystemType string
}

func newBindRequest(s Auth, bindingType pdu.BindingType) (bindReq *pdu.BindRequest) {
	bindReq = pdu.NewBindRequest(bindingType)
	bindReq.SystemID = s.SystemID
	bindReq.Password = s.Password
	bindReq.SystemType = s.SystemType
	return
}

// ConnectAsReceiver connects to SMSC as Receiver.
func ConnectAsReceiver(dialer Dialer, s Auth) (*Connection, error) {
	return connectAs(pdu.Receiver, dialer, s)
}

// ConnectAsTransmitter connects to SMSC as Transmitter.
func ConnectAsTransmitter(dialer Dialer, s Auth) (*Connection, error) {
	return connectAs(pdu.Transmitter, dialer, s)
}

// ConnectAsTransceiver connects to SMSC as Transceiver.
func ConnectAsTransceiver(dialer Dialer, s Auth) (*Connection, error) {
	return connectAs(pdu.Transceiver, dialer, s)
}

// connectAs connects to SMSC as specified BindingType.
func connectAs(b pdu.BindingType, dialer Dialer, s Auth) (*Connection, error) {
	return connect(dialer, s.SMSC, newBindRequest(s, b))
}
