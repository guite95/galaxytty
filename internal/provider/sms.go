package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const (
	defaultMessageLimit = 200
	maximumMessageLimit = 1000
	pollMessageLimit    = 500
)

var smsProjection = []string{"_id", "thread_id", "address", "date", "type", "read", "body"}

func (s *Store) Messages(ctx context.Context, threadID int64, messageQuery domain.MessageQuery) ([]domain.Message, error) {
	if threadID <= 0 {
		return nil, fmt.Errorf("thread ID must be positive")
	}
	if messageQuery.BeforeID < 0 {
		return nil, fmt.Errorf("before ID cannot be negative")
	}
	if messageQuery.Limit < 0 {
		return nil, fmt.Errorf("message limit cannot be negative")
	}
	limit := messageQuery.Limit
	if limit == 0 {
		limit = defaultMessageLimit
	}
	if limit > maximumMessageLimit {
		limit = maximumMessageLimit
	}
	where := fmt.Sprintf("thread_id = %d", threadID)
	if messageQuery.BeforeID > 0 {
		where += fmt.Sprintf(" AND _id < %d", messageQuery.BeforeID)
	}
	rows, err := s.query(ctx, Query{
		URI:          "content://sms",
		Projection:   smsProjection,
		Where:        where,
		Sort:         fmt.Sprintf("_id DESC LIMIT %d", limit),
		FreeFormLast: "body",
	})
	if err != nil {
		return nil, err
	}
	messages, err := mapSMSRows(rows)
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, nil
}

func (s *Store) LatestMessageID(ctx context.Context) (int64, error) {
	rows, err := s.query(ctx, Query{
		URI:        "content://sms",
		Projection: []string{"_id"},
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

func (s *Store) MessagesAfter(ctx context.Context, lastID int64) ([]domain.Message, error) {
	if lastID < 0 {
		return nil, fmt.Errorf("last message ID cannot be negative")
	}
	rows, err := s.query(ctx, Query{
		URI:          "content://sms",
		Projection:   smsProjection,
		Where:        fmt.Sprintf("_id > %d", lastID),
		Sort:         fmt.Sprintf("_id ASC LIMIT %d", pollMessageLimit),
		FreeFormLast: "body",
	})
	if err != nil {
		return nil, err
	}
	return mapSMSRows(rows)
}

func mapSMSRows(rows []map[string]string) ([]domain.Message, error) {
	messages := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		message, err := mapSMSRow(row)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func mapSMSRow(row map[string]string) (domain.Message, error) {
	id, err := parseInt64Field(row, "_id")
	if err != nil {
		return domain.Message{}, err
	}
	threadID, err := parseInt64Field(row, "thread_id")
	if err != nil {
		return domain.Message{}, err
	}
	date, err := parseInt64Field(row, "date")
	if err != nil {
		return domain.Message{}, err
	}
	rawType, err := parseIntField(row, "type")
	if err != nil {
		return domain.Message{}, err
	}
	direction, err := DirectionFromAndroid(rawType)
	if err != nil {
		return domain.Message{}, err
	}
	rawRead, err := parseIntField(row, "read")
	if err != nil {
		return domain.Message{}, err
	}
	if rawRead != 0 && rawRead != 1 {
		return domain.Message{}, fmt.Errorf("invalid SMS read flag")
	}
	return domain.Message{
		ID:          id,
		ThreadID:    threadID,
		Address:     row["address"],
		Body:        row["body"],
		Timestamp:   time.UnixMilli(date),
		Direction:   domain.MessageDirection(direction),
		Read:        rawRead == 1,
		Type:        domain.MessageSMS,
		Attachments: nil,
	}, nil
}

func parseInt64Field(row map[string]string, name string) (int64, error) {
	value, ok := row[name]
	if !ok {
		return 0, fmt.Errorf("missing SMS field %s", name)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid SMS field %s", name)
	}
	return parsed, nil
}

func parseIntField(row map[string]string, name string) (int, error) {
	value, ok := row[name]
	if !ok {
		return 0, fmt.Errorf("missing SMS field %s", name)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid SMS field %s", name)
	}
	return parsed, nil
}
