// ------------------
// User: pei
// DateTime: 2020/10/28 10:26
// Description: 
// ------------------

package main

import (
	"bytes"
	"errors"
	"github.com/guzhi17/xcon"
	"log"
	"time"
)

func main() {
	log.SetFlags(11)
	s := xcon.CreateServer(xcon.Config{
		Handler:          &TelnetEchoManager{},
		ConnConfig: xcon.ConnConfig{
			PackageMaxLength: 1 << 10,
			PackageMode:      xcon.PmNone, //Pm32,
			BufferLength:     0,
			DialTimeout:      0,
			ReadTimeout:      time.Second*100,
			WriteTimeout:     time.Second*10,
		},
		Addr:             ":3721",
	})
	err := s.ListenAndServe()
	log.Println(err)
}

//========================================================================================================================
type TelnetEchoManager struct {}
func (h *TelnetEchoManager)OnConn(c *xcon.Conn)(xcon.Session, error){
	log.Printf("[%p,OnConn]\n", c)
	return &Session{conn:c}, nil
}
func (h *TelnetEchoManager)OnClose(c xcon.Session, err error)error{
	switch v := c.(type) {
	default:
		log.Printf("[%p,OnClose[Unknown]%v]\n", c, err)
	case *Session:
		log.Printf("[%p,OnClose]%v\n", v.conn, err)
	}
	return nil
}
//========================================================================================================================
type Session struct {
	conn *xcon.Conn
}
var(
	exit = []byte("exit")
)
func (s *Session)OnData(b []byte)( err error){
	log.Printf("[OnData]: %d-%s\n", len(b), b)
	_, err = s.conn.Write(b, b)
	if err != nil{
		return err
	}
	l := len(b)
	_, err = s.conn.Write(b, b[:l/2])
	if err != nil{
		return err
	}
	time.Sleep(time.Second/2)
	_, err = s.conn.Write(b[l/2:])
	if err != nil{
		return err
	}
	if bytes.Equal(b, exit){
		return errors.New("exit")
	}
	return nil
}
//========================================================================================================================