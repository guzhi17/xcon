// ------------------
// User: pei
// DateTime: 2020/10/28 10:21
// Description: 
// ------------------

package xcon

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestListenAndServe(t *testing.T) {
	t0 := time.Now()
	sum := 0

	for i := 0; i <= 10000000; i++ {
		if isPrime(i) == true {
			sum = sum + 1

		}
	}
	fmt.Println(sum)
	fmt.Println(time.Now().Sub(t0))
}
func isPrime(n int) bool {

	end := int(math.Sqrt(float64(n)))
	for i := 2; i <= end; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}