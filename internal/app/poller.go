package app

import (
	"context"
	"github.com/galaxytty/galaxytty/internal/domain"
	"sort"
)

type Poller struct {
	Store       domain.MessageStore
	LastSeenID  int64
	initialized bool
}

func (p *Poller) Initialize(ctx context.Context) error {
	id, err := p.Store.LatestMessageID(ctx)
	if err != nil {
		return err
	}
	p.LastSeenID, p.initialized = id, true
	return nil
}

func (p *Poller) Poll(ctx context.Context) ([]domain.Message, error) {
	if !p.initialized {
		if err := p.Initialize(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	ms, e := p.Store.MessagesAfter(ctx, p.LastSeenID)
	if e != nil {
		return nil, e
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
	for _, m := range ms {
		if m.ID > p.LastSeenID {
			p.LastSeenID = m.ID
		}
	}
	return ms, nil
}
