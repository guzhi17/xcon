// ------------------
// User: pei
// DateTime: 2020/10/27 15:48
// Description: 
// ------------------

package xcon

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"net"
	"sync"
	"time"
)


type ConnConfig struct {
	PackageMaxLength int
	PackageMode      PackageMode
	BufferLength	int
	DialTimeout time.Duration
	ReadTimeout time.Duration
	WriteTimeout time.Duration
}

// A conn represents the server side of an HTTP connection.
type Conn struct {
	// server is the server on which the connection arrived.
	// Immutable; never nil.
	Config ConnConfig

	// cancelCtx cancels the connection-level context.
	cancelCtx context.CancelFunc

	// rwc is the underlying network connection.
	// This is never wrapped by other types and is the value given out
	// to CloseNotifier callers. It is usually of type *net.TCPConn or
	// *tls.Conn.
	rwc net.Conn
	wmu sync.Mutex

	// remoteAddr is rwc.RemoteAddr().String(). It is not populated synchronously
	// inside the Listener's Accept goroutine, as some implementations block.
	// It is populated immediately inside the (*conn).serve goroutine.
	// This is the value of a Handler's (*Request).RemoteAddr.
	remoteAddr string

	// tlsState is the TLS connection state when using TLS.
	// nil means not TLS.
	tlsState *tls.ConnectionState


	//cd chan []byte
	closed AtomInt32
}

func NewConn(rwc net.Conn, cfg ConnConfig) *Conn {
	c := &Conn{
		Config: cfg,
		rwc:    rwc,
		//cd: make(chan []byte, 1),
	}
	return c
}

func (c *Conn) TlsState() *tls.ConnectionState  {
	return c.tlsState
}
func (c *Conn) RemoteAddr() string  {
	return c.remoteAddr
}
func (c *Conn) Closed() bool  {
	return c.closed.Load() > 0
}

func (c *Conn) Close() error  {
	if c.closed.Inc() > 1{
		return ErrConnClosed
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.rwc.Close()
}


func GetTotalLen(bs... []byte)(total int, none int, single []byte){
	for _, bi := range bs{
		li := len(bi)
		if li < 1{continue}
		total += li
		none += 1
		single = bi
	}
	return
}

func (c *Conn) WriteRaw(b []byte) (n int, err error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if d := c.Config.WriteTimeout; d != 0 {
		c.rwc.SetWriteDeadline(time.Now().Add(d))
	}
	return c.rwc.Write(b)
}

func (c *Conn) Write(bs... []byte) (n int, err error) {
	if c.closed.Load() != 0 {
		return 0, ErrConnClosed
	}
	var pm = c.Config.PackageMode

	//var lenBuf []byte
	totalLen, noneCnt, single := GetTotalLen(bs...)
	if totalLen < 1{
		return 0, nil
	}
	var (
		buffer *Buffer
		pmLen int
	)
	switch pm {
	default:
		return 0, ErrPackageModel
	case PmNone:
		if noneCnt == 1{
			return c.WriteRaw(single)
		}
		//go write directly
		buffer = bufferPool.GetBuffer(totalLen)
	case Pm16:
		if totalLen > 0x7fff{
			return 0, ErrPackageTooLarge
		}
		pmLen = 2
		buffer = bufferPool.GetBuffer(totalLen + pmLen)
		binary.BigEndian.PutUint16(buffer.Data, uint16(totalLen))
	case Pm32:
		if totalLen > 0x7fffffff{
			return 0, ErrPackageTooLarge
		}
		pmLen = 4
		buffer = bufferPool.GetBuffer(totalLen + pmLen)
		binary.BigEndian.PutUint32(buffer.Data, uint32(totalLen))
	}
	defer buffer.Close()
	switch noneCnt {
	default:
		for _, bi := range bs{
			li := len(bi)
			if li < 1{continue}
			copy(buffer.Data[pmLen:], bi)
			pmLen += li
		}
	case 1:
		copy(buffer.Data[pmLen:], single)
		pmLen += totalLen
	}
	return c.WriteRaw(buffer.Data[:pmLen])
}
// Serve a new connection.
func (c *Conn) serve(handler Handler, ctx context.Context) {
	defer c.Close()
	c.remoteAddr = c.rwc.RemoteAddr().String()
	ctx = context.WithValue(ctx, LocalAddrContextKey, c.rwc.LocalAddr())
	defer func() {
		if err := recover(); err != nil && err != ErrAbortHandler {
			//const size = 64 << 10
			//buf := make([]byte, size)
			//buf = buf[:runtime.Stack(buf, false)]
			//c.server.logf("http: panic serving %v: %v\n%s", c.remoteAddr, err, buf)
		}
	}()

	if tlsConn, ok := c.rwc.(*tls.Conn); ok {
		if d := c.Config.ReadTimeout; d != 0 {
			c.rwc.SetReadDeadline(time.Now().Add(d))
		}
		if d := c.Config.WriteTimeout; d != 0 {
			c.rwc.SetWriteDeadline(time.Now().Add(d))
		}
		if err := tlsConn.Handshake(); err != nil {
			//tlsConn.Close() //?
			//c.server.logf("http: TLS handshake error from %s: %v", c.rwc.RemoteAddr(), err)
			return
		}
		c.tlsState = new(tls.ConnectionState)
		*c.tlsState = tlsConn.ConnectionState()
	}
	ctx, cancelCtx := context.WithCancel(ctx)
	c.cancelCtx = cancelCtx
	defer cancelCtx()

	ses, err := handler.OnConn(c)
	if err != nil{
		return
	}

	defer func() {
		handler.OnClose(ses, err)
	}()

	var buffer = MakeReusableBuffer(c.Config.BufferLength)
	var data []byte
	for {
		//this is the reading routing
		data, err = c.ReadRequest(buffer, ctx)
		if err != nil{
			return
		}
		err = ses.OnData(data)
		if err != nil{
			return
		}
		//c.cd <- data
	}
}


func (c *Conn)Serve(ses Session, ctx context.Context) error {
	defer c.Close()
	c.remoteAddr = c.rwc.RemoteAddr().String()
	ctx = context.WithValue(ctx, LocalAddrContextKey, c.rwc.LocalAddr())
	defer func() {
		if err := recover(); err != nil && err != ErrAbortHandler {
			//const size = 64 << 10
			//buf := make([]byte, size)
			//buf = buf[:runtime.Stack(buf, false)]
			//c.server.logf("http: panic serving %v: %v\n%s", c.remoteAddr, err, buf)
		}
	}()
	if tlsConn, ok := c.rwc.(*tls.Conn); ok {
		c.tlsState = new(tls.ConnectionState)
		*c.tlsState = tlsConn.ConnectionState()
	}
	ctx, cancelCtx := context.WithCancel(ctx)
	c.cancelCtx = cancelCtx
	defer cancelCtx()

	var buffer = MakeReusableBuffer(c.Config.BufferLength)
	var (
		data []byte
		err error
	)
	for {
		//this is the reading routing
		data, err = c.ReadRequest(buffer, ctx)
		if err != nil{
			return err
		}
		err = ses.OnData(data)
		if err != nil{
			return err
		}
		//c.cd <- data
	}
}


type ReusableBuffer struct {
	b []byte
	sz int
}

func MakeReusableBuffer(sz int) (r ReusableBuffer) {
	r.b = make([]byte, sz)
	r.sz = sz
	return
}


type ReadConfig struct {
	PackageMaxLength int
	PackageMode      PackageMode
	BufferLength	int
	ReadTimeout time.Duration
}
func (c *Conn)ReadRequest(buf ReusableBuffer, ctx context.Context) (data []byte, err error) {
	var (
		hdrDeadline      time.Time // or zero if none
	)
	if d := c.Config.ReadTimeout; d != 0 {
		t0 := time.Now()
		hdrDeadline = t0.Add(d)
		c.rwc.SetReadDeadline(hdrDeadline)
	}

	switch c.Config.PackageMode {
	case PmNone:
		n, err := c.rwc.Read(buf.b)
		if err != nil{
			//log.Println(err)
			return nil, err
		}
		return buf.b[:n], nil
	case Pm16:
		var lenBuf = []byte{0,0}
		_, err = readFull(c.rwc, lenBuf, 2)
		if err != nil{
			return
		}
		lenData := binary.BigEndian.Uint16(lenBuf)
		if int(lenData) > c.Config.PackageMaxLength{
			return nil, ErrPackageTooLarge
		}
		data, err = ReadUtil(c.rwc, int(lenData), buf)
		if err != nil{
			return
		}
	case Pm32:
		var lenBuf = []byte{0,0, 0,0}
		_, err = readFull(c.rwc, lenBuf, 4)
		if err != nil{
			return
		}
		lenData := binary.BigEndian.Uint32(lenBuf)
		if int(lenData) > c.Config.PackageMaxLength{
			return nil, ErrPackageTooLarge
		}
		data, err = ReadUtil(c.rwc, int(lenData), buf)
		if err != nil{
			return
		}
	}

	return
}
func readFull(conn net.Conn, resp []byte, sz int) ([]byte, error) {
	pt := 0
	for{
		n, err := conn.Read(resp[pt:])
		if err != nil{
			//log.Println(err)
			return nil, err
		}
		pt += n
		if pt >= sz{
			break
		}
	}
	return resp[:sz], nil
}
func ReadUtil(conn net.Conn, sz int, r ReusableBuffer) ([]byte, error) {
	//% todo set timeout
	if r.sz >= sz{
		return readFull(conn, r.b, sz)
	}else{
		resp := make([]byte, sz)
		return readFull(conn, resp, sz)
	}
}