package provider

import (
	"context"
	"sync"
)

type Sheller interface {
	Shell(context.Context, ...string) ([]byte, error)
}

type Store struct {
	shell Sheller

	contactsMu     sync.Mutex
	contactsLoaded bool
	contacts       map[string]string
}

func NewStore(shell Sheller) *Store {
	return &Store{shell: shell}
}
