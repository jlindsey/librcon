package rcon

import (
	"net"

	"github.com/pkg/errors"
)

// Login logs in
func Login(addr, password string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	p, err := NewPacket(typeLogin, password)
	if err != nil {
		return errors.Wrap(err, "error creating new packet struct")
	}
	buf, err := p.Encode()
	if err != nil {
		return errors.Wrap(err, "error encoding packet")
	}

	_, err = conn.Write(buf)
	if err != nil {
		return errors.Wrap(err, "error writing to tcp conn")
	}

	respBuf := make([]byte, 4096)
	_, err = conn.Read(respBuf)
	if err != nil {
		return errors.Wrap(err, "error reading from tcp conn")
	}
	respPacket, err := Decode(respBuf)
	if err != nil {
		return errors.Wrap(err, "error decoding response packet")
	}

	if respPacket.ID == -1 {
		return errors.New("bad password or login error")
	}

	return nil
}
