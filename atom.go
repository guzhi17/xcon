package xcon

import "sync/atomic"

// Int32 is an atomic wrapper around an int32.
type AtomInt32 int32

// Load atomically loads the wrapped value.
func (i *AtomInt32) Load() int32 {
	return atomic.LoadInt32((*int32)(i))
}

// Add atomically adds to the wrapped int32 and returns the new value.
func (i *AtomInt32) Add(n int32) int32 {
	return atomic.AddInt32((*int32)(i), n)
}

// Sub atomically subtracts from the wrapped int32 and returns the new value.
func (i *AtomInt32) Sub(n int32) int32 {
	return atomic.AddInt32((*int32)(i), -n)
}

// Inc atomically increments the wrapped int32 and returns the new value.
func (i *AtomInt32) Inc() int32 {
	return i.Add(1)
}

// Dec atomically decrements the wrapped int32 and returns the new value.
func (i *AtomInt32) Dec() int32 {
	return i.Sub(1)
}

// CAS is an atomic compare-and-swap.
func (i *AtomInt32) CAS(old, new int32) bool {
	return atomic.CompareAndSwapInt32((*int32)(i), old, new)
}

// Store atomically stores the passed value.
func (i *AtomInt32) Store(n int32) {
	atomic.StoreInt32((*int32)(i), n)
}

// Swap atomically swaps the wrapped int32 and returns the old value.
func (i *AtomInt32) Swap(n int32) int32 {
	return atomic.SwapInt32((*int32)(i), n)
}