package xcon

import "sync"

const(
	PoolBufferMaxSize = 512
)
//var bufferPool = &sync.Pool{
//	New: func() interface{} {
//		return make([]byte, PoolBufferMaxSize)
//	},
//}

type Buffer struct {
	Data []byte
	Owner *BytesPool
}

func (s *Buffer)Close()  {
	if s.Owner != nil{
		s.Owner.ReturnBuffer(s)
	}
}

type BytesPool struct {
	size int
	pool *sync.Pool
}

func (s BytesPool)GetBuffer(sz int) *Buffer {
	if sz > s.size{
		return &Buffer{Data: make([]byte, sz)}
	}
	return s.pool.Get().(*Buffer)
}
func (s *BytesPool)ReturnBuffer(b *Buffer)  {
	if b != nil{
		s.pool.Put(b)
	}
}

var bufferPool = NewBytesPool(PoolBufferMaxSize)

func NewBytesPool(sz int) *BytesPool {
	s := &BytesPool{
		size: sz,
		pool: &sync.Pool{
		},
	}
	s.pool.New = func() interface{} {
		return &Buffer{Data: make([]byte, PoolBufferMaxSize), Owner: s}
	}
	return s
}