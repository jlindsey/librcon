package rcon

import (
	"bytes"
	"net"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

var (
	// Log is the logging interface for the library. All outputs are
	// Debug or Trace and the logger is set to INFO by default, so
	// you need to reset the level to see any output.
	Log hclog.Logger = hclog.New(&hclog.LoggerOptions{
		Name:            "librcon",
		Level:           hclog.Info,
		IncludeLocation: true,
	})
)

type queueEntry struct {
	id   int32
	kind PacketType
	cmd  string
}

type futureChans struct {
	out chan<- string
	err chan<- error
}

func (f *futureChans) Close() error {
	close(f.out)
	close(f.err)
	return nil
}

// Client exposes the rcon api
type Client struct {
	conn        *net.TCPConn
	pw          string
	queue       chan queueEntry
	pending     map[int32]*futureChans
	pendingM    sync.RWMutex
	loginID     int32
	loginFuture *futureChans
	nextID      int32
	idM         sync.Mutex

	Error chan error
}

// NewClient returns a new client
func NewClient(addr, pw string) (*Client, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve address")
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, errors.Wrap(err, "error while dialing tcp")
	}

	c := &Client{
		conn:    conn,
		pw:      pw,
		queue:   make(chan queueEntry, 32),
		pending: make(map[int32]*futureChans),
		nextID:  1,
		Error:   make(chan error, 5),
	}

	go c.reader()
	go c.writer()

	return c, nil
}

// Close implements the Closer interface
func (c *Client) Close() error {
	var result error

	if err := c.conn.Close(); err != nil {
		result = multierror.Append(result, err)
	}

	close(c.queue)
	close(c.Error)

	for _, f := range c.pending {
		if err := f.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

func (c *Client) login() error {
	Log.Debug("Logging in")
	future := c.SubmitCommandWithType(c.pw, typeLogin)
	c.loginID = future.ID

	defer func() {
		c.loginFuture = nil
	}()

	select {
	case err := <-future.Error:
		return err
	case err := <-c.Error:
		return err
	case <-future.Return:
		return nil
	}
}

// SubmitCommand adds a command to the queue with the default type (Command) and returns a future.
// This method may block if the queue is full.
func (c *Client) SubmitCommand(cmd string) *CommandFuture {
	return c.SubmitCommandWithType(cmd, typeCommand)
}

// SubmitCommandWithType adds a command to the queue of a specific type and returns a future. This
// method may block if the queue is full.
func (c *Client) SubmitCommandWithType(cmd string, t PacketType) *CommandFuture {
	id := c.getNextID()
	errChan := make(chan error)
	outChan := make(chan string)

	entry := queueEntry{
		id:   id,
		kind: t,
		cmd:  cmd,
	}

	future := &futureChans{
		err: errChan,
		out: outChan,
	}

	if t == typeLogin {
		c.loginFuture = future
	} else {
		c.pendingM.Lock()
		c.pending[id] = future
		c.pendingM.Unlock()
	}

	c.queue <- entry
	Log.Trace("Queued new comamnd", "entry", hclog.Fmt("%#v", entry))

	return &CommandFuture{
		ID:      id,
		Command: cmd,
		Error:   errChan,
		Return:  outChan,
	}
}

func (c *Client) reader() {
	var (
		buf                  bytes.Buffer
		currPacketSize, read int32
		currPacket           []byte
	)

	logger := Log.Named("reader")

	for {
		readBuf := make([]byte, 4096)
		logger.Trace("Reader waiting for connection data")
		n, err := c.conn.Read(readBuf)
		logger.Trace("Reader got bytes", "n", n)
		if err != nil {
			c.Error <- errors.Wrap(err, "error reading from connection")
			continue
		}
		if n == 0 {
			continue
		}

		x, err := buf.Write(readBuf[0:n])
		if err != nil {
			c.Error <- errors.Wrapf(err, "error copying into buffer after %d bytes", x)
			continue
		}

		if currPacketSize == 0 {
			if buf.Len() < 4 {
				continue
			}

			sizeBuf := buf.Next(4)

			currPacketSize, err = readInt(bytes.NewBuffer(sizeBuf))
			if err != nil {
				c.Error <- errors.Wrap(err, "error reading current packet size")
				continue
			}
			currPacketSize += 4
			logger.Trace("Read packet size", "currPacketSize", currPacketSize)

			read = 4
			currPacket = make([]byte, currPacketSize)
			for i, b := range sizeBuf {
				currPacket[i] = b
			}
		}

		if read < currPacketSize && buf.Len() > 0 {
			currIterBuf := make([]byte, currPacketSize-read)
			n, err := buf.Read(currIterBuf)
			read += int32(n)

			if err != nil {
				c.Error <- errors.Wrap(err, "error writing into iteration buffer")
				continue
			}

			for i, b := range currIterBuf {
				currPacket[i+4] = b
			}
		}

		if read == currPacketSize {
			logger.Trace("Probably have complete packet", "read", read, "currPacketSize", currPacketSize)
			respPacket, err := Decode(currPacket)
			logger.Trace("Reader decoded packet", "respPacket", respPacket)

			currPacketSize = 0
			read = 0
			currPacket = nil

			if err != nil {
				c.Error <- errors.Wrap(err, "error decoding packet")
				continue
			}

			id := respPacket.ID
			if id == -1 {
				c.loginFuture.err <- errors.Errorf("bad login")
				continue
			}

			c.pendingM.RLock()
			futures, ok := c.pending[id]
			c.pendingM.RUnlock()
			if !ok {
				if id == c.loginID && c.loginFuture != nil {
					c.loginFuture.out <- respPacket.Payload
					c.loginFuture.Close()
					continue
				}
				c.Error <- errors.Errorf("decoded packet with no associated pending future: %#v", respPacket)
				continue
			}

			futures.out <- respPacket.Payload
			futures.Close()
		}
	}
}

func (c *Client) writer() {
	logger := Log.Named("writer")

	for {
		logger.Trace("Waiting for queue")
		item := <-c.queue
		logger = logger.With("item", hclog.Fmt("%#v", item))
		logger.Trace("Got queue item")

		var futures *futureChans

		if item.kind == typeLogin {
			futures = c.loginFuture
		} else {
			c.pendingM.RLock()
			var ok bool
			futures, ok = c.pending[item.id]
			c.pendingM.RUnlock()
			if !ok {
				c.Error <- errors.Errorf("packet in queue with no pending future: %#v", item)
				continue
			}
		}

		packet, err := NewPacket(item.id, item.kind, item.cmd)
		logger.Trace("Packet from queue item", "packet", packet)
		if err != nil {
			futures.err <- errors.Wrap(err, "unable to construct new packet")
			continue
		}

		packetBytes, err := packet.Encode()
		if err != nil {
			futures.err <- errors.Wrap(err, "unable to encode packet to binary")
			continue
		}

		packetBuf := bytes.NewBuffer(packetBytes)
		_, err = packetBuf.WriteTo(c.conn)
		if err != nil {
			futures.err <- errors.Wrap(err, "unable to write packet to connection")
			continue
		}
		logger.Trace("Packet written to connection", "packet", hclog.Fmt("%#v", packetBuf))
	}
}

func (c *Client) getNextID() int32 {
	c.idM.Lock()
	defer c.idM.Unlock()

	id := c.nextID
	c.nextID++
	return id
}
