package xdag

import (
	"context"
	"errors"
	"github.com/swordlet/xmrig2xdag/logger"
	"io"
	"net"
	"sync"
	"time"
)

//Connection to XDAG pool
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
	done      chan int
}

func NewConnection(conn net.Conn, connID uint64, notify chan []byte, done chan int) *Connection {
	//initialization
	c := &Connection{
		Conn:        conn,
		ConnID:      connID,
		isClosed:    false,
		msgBuffChan: make(chan []byte, 8),
		jobNotify:   notify,
		done:        done,
	}

	return c
}

//StartWriter write message Goroutine, send message to XDAG pool
func (c *Connection) StartWriter() {
	logger.Get().Println("[Writer Goroutine is running]")
	defer logger.Get().Println(c.RemoteAddr().String(), "[conn Writer exit!]")

	for {
		select {
		case data, ok := <-c.msgBuffChan:
			if ok {
				if _, err := c.Conn.Write(data); err != nil {
					logger.Get().Println("Send Buff Data error:, ", err, " Conn Writer exit")
					return
				}
			} else {
				logger.Get().Println("msgBuffChan is Closed")
				break
			}
		case <-c.ctx.Done():
			return
		}
	}
}

//StartReader read message Goroutine, receive message from XDAG pool
func (c *Connection) StartReader() {
	logger.Get().Println("[Reader Goroutine is running]")
	defer logger.Get().Println(c.RemoteAddr().String(), "[conn Reader exit!]")
	defer c.Stop()

	for {
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
				return
			}

			c.jobNotify <- data
		}
	}
}

//Start a connection
func (c *Connection) Start() {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	//1 start receive Goroutine
	go c.StartReader()
	//2 start send Goroutine
	go c.StartWriter()
}

//Stop a connection
func (c *Connection) Stop() {

	c.Lock()
	defer c.Unlock()

	if c.isClosed == true {
		return
	}

	logger.Get().Println("Conn Stop()...ConnID = ", c.ConnID)

	c.Conn.Close()
	//close writer
	c.cancel()

	//close channel
	close(c.msgBuffChan)
	//set flag
	c.isClosed = true
	c.done <- 1

}

//GetTCPConnection get socket TCPConn
func (c *Connection) GetTCPConnection() net.Conn {
	return c.Conn
}

//GetConnID  get ID
func (c *Connection) GetConnID() uint64 {
	return c.ConnID
}

//RemoteAddr get remote address
func (c *Connection) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

//SendBuffMsg  send message to XDAG pool through buffered channel
func (c *Connection) SendBuffMsg(data []byte) error {
	c.RLock()
	defer c.RUnlock()
	if c.isClosed == true {
		return errors.New("Connection closed when send buff msg")
	}
	c.msgBuffChan <- data

	return nil
}

func (c *Connection) SendLogin() error {
	return nil
}
