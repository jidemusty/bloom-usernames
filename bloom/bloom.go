package bloom

import (
	"hash/fnv"
	"math"
	"sync"
)

type Filter struct {
	boxes []uint8
	m     int
	k     int
	mu    sync.RWMutex
}

// creating a struct is usually done by a constructor function named New...
// &Filter{...} means "build a Filter and give me a pointer to it"
func NewFilter(m int, k int) *Filter {
	return &Filter{
		boxes: make([]uint8, (m+7)/8),
		m:     m,
		k:     k,
	}
}

func NewFilterFor(n int, p float64) *Filter {
	m, k := OptimalParams(n, p)
	return NewFilter(m, k)
}

func (f *Filter) positions(name string) []int {
	// hash the name, return sum % m as an int
	h := fnv.New64a()
	h.Write([]byte(name))
	sum := h.Sum64() // uint64

	h1 := sum & 0xFFFFFFFF
	h2 := (sum >> 32) | 1

	out := make([]int, f.k)

	for i := range f.k {
		out[i] = int((h1 + uint64(i)*h2) % uint64(f.m))
	}

	return out
}

// functions attached to a struct are methods
// *Filter - a pointer: "the actual filter, not a copy".
// without the *, Go would hand the method a copy, and changes would vanish.

// JS objects are always passed by reference; Go makes you choose

// capital letter means public to other packages. Lowercase is private.
// That is the whole visibility system.
func (f *Filter) Add(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.positions(name) {
		f.boxes[p/8] |= 1 << (p % 8)
	}
}

func (f *Filter) MightContain(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// return false if any position is off, otherwise true
	for _, p := range f.positions(name) {
		// box is p/8, switch position is p%8
		if f.boxes[p/8]&(1<<(p%8)) == 0 {
			return false
		}
	}

	return true
}

func OptimalParams(n int, p float64) (m int, k int) {
	// Formula for m: -n * ln(p) / (ln(2)^2)
	mFloat := -float64(n) * math.Log(p) / math.Pow(math.Ln2, 2)
	m = int(math.Ceil(mFloat))

	// Formula for k: (m / n) * ln(2)
	kFloat := (float64(m) / float64(n)) * math.Ln2
	k = int(math.Round(kFloat))

	return m, k
}
