package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestOptimalParams(t *testing.T) {
	m, k := OptimalParams(100_000, 0.01)
	t.Logf("m=%d k=%d", m, k)

	if m < 958_000 || m > 959_000 {
		t.Errorf("m = %d, want about 958,506", m)
	}
	if k != 7 {
		t.Errorf("k = %d, want 7", k)
	}
}

func TestNoFalseNegatives(t *testing.T) {
	f := NewFilterFor(100_000, 0.01)

	for i := range 100_000 {
		name := fmt.Sprintf("user%d", i)
		f.Add(name)
	}

	for i := range 100_000 {
		name := fmt.Sprintf("user%d", i)
		if !f.MightContain(name) {
			t.Fatalf("failed for %v", name)
		}
	}
}

func TestFalsePositiveRate(t *testing.T) {
	f := NewFilterFor(100_000, 0.01)

	for i := range 100_000 {
		name := fmt.Sprintf("user%d", i)
		f.Add(name)
	}

	count := 0

	for i := 100_000; i < 200_000; i++ {
		name := fmt.Sprintf("user%d", i)

		if f.MightContain(name) {
			count++
		}
	}

	rate := float64(count) / 100_000

	t.Logf("false positive rate: %.4f", rate)

	if rate > 0.015 {
		t.Fatalf("failed for %.4f", rate)
	}
}

func TestConcurrentAdd(t *testing.T) {
	f := NewFilterFor(100_000, 0.01)

	var wg sync.WaitGroup

	for g := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := g; i < 100_000; i += 8 {
				f.Add(fmt.Sprintf("user%d", i))
			}
		}()
	}

	wg.Wait()

	for i := range 100_000 {
		name := fmt.Sprintf("user%d", i)
		if !f.MightContain(name) {
			t.Fatalf("failed for %v", name)
		}
	}
}
