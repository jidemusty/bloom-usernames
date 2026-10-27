package checker

import "context"

type Checker interface {
	MightContain(ctx context.Context, name string) (bool, error)
	Add(ctx context.Context, name string) error
	Name() string
}
