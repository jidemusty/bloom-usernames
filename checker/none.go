package checker

import "context"

type None struct{}

func (None) MightContain(ctx context.Context, name string) (bool, error) {
	return true, nil
}

func (None) Add(ctx context.Context, name string) error {
	return nil
}

func (None) Name() string {
	return "none"
}
