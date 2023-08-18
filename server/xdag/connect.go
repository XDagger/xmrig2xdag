package xdag

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swordlet/xmrig2xdag/logger"
)

var PoolDown atomic.Uint64

// Connection to XDAG pool
type Connection struct {
	sync.RWMutex
	Conn        net.Conn           //tcp socket
	ConnID      uint64             //ID
	ctx         context.Context    //exit channel
	cancel      context.CancelFunc //cancel channel
	msgBuffChan chan []byte        //buffered message channel
	isClosed    bool               //closed flag
	//isMulti     bool               // multiple works
	//worksCounts int
	jobNotify chan []byte
	// done      chan int
	EOFcount atomic.Uint64
}

func NewConnection(conn net.Conn, connID uint64, notify chan []byte, done chan int) *Connection {
	//initialization
	c := &Connection{
		Conn:        conn,
		ConnID:      connID,
		isClosed:    false,
		msgBuffChan: make(chan []byte, 8),
		jobNotify:   notify,
		// done:        done,
	}

	return c
}

// StartWriter write message Goroutine, send message to XDAG pool
func (c *Connection) StartWriter() {
	logger.Get().Debugln("[Writer Goroutine is running]")
	defer logger.Get().Debugln(c.RemoteAddr().String(), "[conn Writer exit!]")
	defer c.Stop()

	for {
		if PoolDown.Load() > 0 {
			return
		}
		select {
		case data, ok := <-c.msgBuffChan:
			if ok {
				if _, err := c.Conn.Write(data); err != nil {
					logger.Get().Println("Send Buff Data error:, ", err, " Conn Writer exit")
					return
				}
			} else {
				logger.Get().Println("msgBuffChan is Closed")
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// StartReader read message Goroutine, receive message from XDAG pool
func (c *Connection) StartReader() {
	logger.Get().Debugln("[Reader Goroutine is running]")
	defer logger.Get().Debugln(c.RemoteAddr().String(), "[conn Reader exit!]")
	defer c.Stop()

	for {
		if PoolDown.Load() > 0 {
			return
		}
		select {
		case <-c.ctx.Done():
			return
		default:
			// 设定连接的等待时长期限
			err := c.Conn.SetReadDeadline(time.Now().Add(time.Second * 128))
			if err != nil {
				return
			}
			data := make([]byte, 32)

			if _, err = io.ReadFull(c.Conn, data); err != nil {
				logger.Get().Println("read msg head error ", err)

				switch errType := err.(type) {
				case net.Error:
					if errType.Timeout() {
						PoolDown.Add(1)
					}
				}

				if err == io.EOF {
					c.EOFcount.Add(1)
				}
				return
			}
			//logger.Get().Debugf("%#v\n", data)
			c.RLock()
			if !c.isClosed {
				c.jobNotify <- data
			}
			c.RUnlock()
		}
	}
}

// Start a connection
func (c *Connection) Start() {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	//1 start receive Goroutine
	go c.StartReader()
	//2 start send Goroutine
	go c.StartWriter()
}

// Stop a connection
func (c *Connection) Stop() {

	c.Lock()
	defer c.Unlock()

	if c.isClosed {
		return
	}

	logger.Get().Println("Conn Stoped()...ConnID = ", c.ConnID)

	c.Conn.Close()
	//close writer
	c.cancel()

	//close channel
	close(c.msgBuffChan)
	//set flag
	c.isClosed = true
	// if PoolDown.Load() > 0 {
	// 	c.done <- 2
	// } else {
	// 	c.done <- 1
	// }

}

// worker disconnected
func (c *Connection) Close() {

	c.Lock()
	defer c.Unlock()

	if c.isClosed {
		return
	}

	logger.Get().Println("Conn Closed() ...ConnID = ", c.ConnID)

	c.Conn.Close()
	//close writer
	c.cancel()

	//close channel
	close(c.msgBuffChan)
	//set flag
	c.isClosed = true

}

// GetTCPConnection get socket TCPConn
func (c *Connection) GetTCPConnection() net.Conn {
	return c.Conn
}

// GetConnID  get ID
func (c *Connection) GetConnID() uint64 {
	return c.ConnID
}

// RemoteAddr get remote address
func (c *Connection) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

// SendBuffMsg  send message to XDAG pool through buffered channel
func (c *Connection) SendBuffMsg(data []byte) error {
	c.RLock()
	defer c.RUnlock()
	if c.isClosed {
		return errors.New("Connection closed when send buff msg")
	}
	c.msgBuffChan <- data

	return nil
}

func (c *Connection) SendLogin() error {
	return nil
}
