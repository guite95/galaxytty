package provider

import "context"

type Sheller interface {
	Shell(context.Context, ...string) ([]byte, error)
}

type Store struct {
	shell Sheller
}

func NewStore(shell Sheller) *Store {
	return &Store{shell: shell}
}
