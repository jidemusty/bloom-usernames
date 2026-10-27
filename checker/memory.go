package checker

import (
	"context"

	"github.com/jidemusty/bloom-usernames/bloom"
)

type Memory struct {
	f *bloom.Filter
}

func NewMemory(f *bloom.Filter) *Memory {
	return &Memory{f: f}
}

func (m *Memory) MightContain(ctx context.Context, name string) (bool, error) {
	return m.f.MightContain(name), nil
}

func (m *Memory) Add(ctx context.Context, name string) error {
	m.f.Add(name)
	return nil
}

func (m *Memory) Name() string {
	return "memory"
}
