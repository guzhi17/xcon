package xcon

import (
	"log"
	"testing"
)

func TestNewBytesPool(t *testing.T) {
	p := NewBytesPool(32)
	{
		b := p.GetBuffer(16)
		b.Close()
	}
	{
		b := p.GetBuffer(33)
		b.Close()
	}
	log.Println("")
}