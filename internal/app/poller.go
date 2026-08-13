package app

import (
	"context"
	"github.com/galaxytty/galaxytty/internal/domain"
	"sort"
)

type Poller struct {
	Store      domain.MessageStore
	LastSeenID int64
}

func (p *Poller) Poll(ctx context.Context) ([]domain.Message, error) {
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
