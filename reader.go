package xcon

import "io"

//Read(p []byte) (n int, err error)
type Reader struct {
	ds [][]byte
	i,j int //
}

func NewReader(bs ...[]byte) *Reader {
	return &Reader{
		ds:    bs,
		i:     0,
		j:     0,
	}
}

func (r *Reader)Read(p []byte) (n int, err error){
	var (
		dn = len(p)
	)
	for{
		if r.i >= len(r.ds){
			err = io.EOF
			return n, err
		}
		dc := r.ds[r.i][r.j:]
		dcn := len(dc)
		if dcn < 1{
			r.j = 0
			r.i += 1
		}
		copy(p[n:], dc)
		if dn-n < dcn{
			r.j = r.j + dn-n
			return dn, nil
		}
		r.j = 0
		r.i += 1
		if dn - n == dcn{
			if r.i >= len(r.ds){
				err = io.EOF
			}
			return dn, err
		}
		n += dcn
	}
}