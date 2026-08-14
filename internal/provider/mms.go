package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const (
	mmsMessageBoxSent = 2
	mmsSendRequest    = 128
	mmsAddressTypeTo  = 151
	mmsPollLimit      = 100
)

var mmsProjection = []string{"_id", "thread_id", "date", "msg_box", "m_type"}

func (s *Store) LatestMMSMessageID(ctx context.Context) (int64, error) {
	rows, err := s.query(ctx, Query{
		URI:        "content://mms",
		Projection: []string{"_id"},
		Where:      "msg_box = 2 AND m_type = 128",
		Sort:       "_id DESC LIMIT 1",
	})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return parseInt64Field(rows[0], "_id")
}

func (s *Store) MMSMessagesAfter(ctx context.Context, lastID int64) ([]domain.Message, error) {
	if lastID < 0 {
		return nil, fmt.Errorf("last MMS ID cannot be negative")
	}
	rows, err := s.query(ctx, Query{
		URI:        "content://mms",
		Projection: mmsProjection,
		Where:      fmt.Sprintf("_id > %d AND msg_box = 2 AND m_type = 128", lastID),
		Sort:       fmt.Sprintf("_id ASC LIMIT %d", mmsPollLimit),
	})
	if err != nil {
		return nil, err
	}
	messages := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		box, err := parseIntField(row, "msg_box")
		if err != nil {
			return nil, err
		}
		messageType, err := parseIntField(row, "m_type")
		if err != nil {
			return nil, err
		}
		if box != mmsMessageBoxSent || messageType != mmsSendRequest {
			continue
		}
		message, complete, err := s.mapOutgoingTextMMS(ctx, row)
		if err != nil {
			return nil, err
		}
		if complete {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (s *Store) mapOutgoingTextMMS(ctx context.Context, row map[string]string) (domain.Message, bool, error) {
	id, err := parseInt64Field(row, "_id")
	if err != nil {
		return domain.Message{}, false, err
	}
	threadID, err := parseInt64Field(row, "thread_id")
	if err != nil {
		return domain.Message{}, false, err
	}
	date, err := parseInt64Field(row, "date")
	if err != nil {
		return domain.Message{}, false, err
	}
	addresses, err := s.query(ctx, Query{
		URI:          fmt.Sprintf("content://mms/%d/addr", id),
		Projection:   []string{"type", "address"},
		Where:        "type = 151",
		FreeFormLast: "address",
	})
	if err != nil {
		return domain.Message{}, false, err
	}
	if len(addresses) != 1 || strings.TrimSpace(addresses[0]["address"]) == "" {
		return domain.Message{}, false, nil
	}
	addressType, err := parseIntField(addresses[0], "type")
	if err != nil {
		return domain.Message{}, false, err
	}
	if addressType != mmsAddressTypeTo {
		return domain.Message{}, false, nil
	}

	parts, err := s.query(ctx, Query{
		URI:          "content://mms/part",
		Projection:   []string{"_id", "mid", "ct", "text"},
		Where:        fmt.Sprintf("mid = %d AND ct = 'text/plain'", id),
		Sort:         "_id ASC",
		FreeFormLast: "text",
	})
	if err != nil {
		return domain.Message{}, false, err
	}
	if len(parts) != 1 || parts[0]["ct"] != "text/plain" || strings.TrimSpace(parts[0]["text"]) == "" {
		return domain.Message{}, false, nil
	}
	partMessageID, err := parseInt64Field(parts[0], "mid")
	if err != nil {
		return domain.Message{}, false, err
	}
	if partMessageID != id {
		return domain.Message{}, false, nil
	}
	return domain.Message{
		ID:        id,
		ThreadID:  threadID,
		Address:   addresses[0]["address"],
		Body:      parts[0]["text"],
		Timestamp: time.Unix(date, 0),
		Direction: domain.DirectionOutgoing,
		Read:      true,
		Type:      domain.MessageMMS,
	}, true, nil
}

var _ domain.MMSMessageStore = (*Store)(nil)
