package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/pkg/errors"
)

//go:generate stringer -type=PacketType

// PacketType is the packet type
type PacketType int32

const (
	typeLogin    PacketType = 3
	typeCommand  PacketType = 2
	typeResponse PacketType = 0
	typeError    PacketType = -1
)

const packetSize = 4096

// Packet is the packet type
type Packet struct {
	ID      int32
	Type    PacketType
	Payload string
}

// NewPacket returns a new Packet
func NewPacket(id int32, packetType PacketType, payload string) (*Packet, error) {
	if packetType != 3 && packetType != 2 {
		return nil, errors.New("invalid packet type, must be 2 or 3")
	}

	packet := Packet{
		ID:      id,
		Type:    packetType,
		Payload: payload,
	}

	return &packet, nil
}

// Decode returns a new Packet from the wire format
func Decode(b []byte) (*Packet, error) {
	packet := Packet{}

	buf := bytes.NewBuffer(b)
	packetLen, err := readInt(buf)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding packet length")
	}

	packet.ID, err = readInt(buf)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding packet id")
	}

	t, err := readInt(buf)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding packet type")
	}
	packet.Type = PacketType(t)

	packetLen = packetLen - 4 - 4 - 2
	strBuf := make([]byte, packetLen)
	_, err = buf.Read(strBuf)
	if err != nil {
		return nil, errors.Wrap(err, "error reading packet payload")
	}
	packet.Payload = string(strBuf)

	return &packet, nil
}

// Encode transforms the packet into the binary wire format
func (p *Packet) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	size := int32(4 + 4 + len(p.Payload) + 2)

	err := binary.Write(buf, binary.LittleEndian, size)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding packet size")
	}

	err = binary.Write(buf, binary.LittleEndian, p.ID)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding packet id")
	}
	err = binary.Write(buf, binary.LittleEndian, p.Type)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding packet type")
	}
	_, err = buf.WriteString(p.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "error writing packet payload")
	}
	_, err = buf.Write([]byte{0x0, 0x0})
	if err != nil {
		return nil, errors.Wrap(err, "error writing packet null pad")
	}

	return buf.Bytes(), nil
}

func (p *Packet) String() string {
	return fmt.Sprintf("Packet{ ID:%d, Type:%s, Payload:%#v }", p.ID, p.Type, p.Payload)
}

func readInt(buf *bytes.Buffer) (int32, error) {
	var out int32

	intBuf := buf.Next(4)
	err := binary.Read(bytes.NewReader(intBuf), binary.LittleEndian, &out)
	if err != nil {
		return 0, err
	}

	return out, nil
}
