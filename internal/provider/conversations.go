package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

var conversationProjection = []string{"_id", "recipient_ids", "unread_count", "date", "snippet"}

func (s *Store) Conversations(ctx context.Context) ([]domain.Conversation, error) {
	conversationRows, err := s.query(ctx, Query{
		URI:          "content://mms-sms/conversations?simple=true",
		Projection:   conversationProjection,
		Sort:         "date DESC",
		FreeFormLast: "snippet",
	})
	if err != nil {
		return nil, err
	}
	canonical, err := s.canonicalAddresses(ctx)
	if err != nil {
		return nil, err
	}
	contacts, err := s.contactNames(ctx)
	if err != nil {
		return nil, err
	}

	conversations := make([]domain.Conversation, 0, len(conversationRows))
	for _, row := range conversationRows {
		conversation, err := mapConversationRow(row, canonical, contacts)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, nil
}

func (s *Store) canonicalAddresses(ctx context.Context) (map[int64]string, error) {
	rows, err := s.query(ctx, Query{
		URI:          "content://mms-sms/canonical-addresses",
		Projection:   []string{"_id", "address"},
		FreeFormLast: "address",
	})
	if err != nil {
		return nil, err
	}
	addresses := make(map[int64]string, len(rows))
	for _, row := range rows {
		id, err := parseConversationInt64(row, "_id")
		if err != nil {
			return nil, err
		}
		addresses[id] = strings.TrimSpace(row["address"])
	}
	return addresses, nil
}

func (s *Store) contactNames(ctx context.Context) (map[string]string, error) {
	s.contactsMu.Lock()
	defer s.contactsMu.Unlock()
	if s.contactsLoaded {
		return s.contacts, nil
	}
	rows, err := s.query(ctx, Query{
		URI:          "content://com.android.contacts/data/phones",
		Projection:   []string{"data1", "data4", "display_name"},
		FreeFormLast: "display_name",
	})
	if err != nil {
		return nil, err
	}
	contacts := make(map[string]string)
	for _, row := range rows {
		name := strings.TrimSpace(row["display_name"])
		for _, field := range []string{"data1", "data4"} {
			phone := domain.NormalizePhone(row[field])
			if phone != "" && name != "" {
				contacts[phone] = name
			}
		}
	}
	s.contacts = contacts
	s.contactsLoaded = true
	return s.contacts, nil
}

func mapConversationRow(row map[string]string, canonical map[int64]string, contacts map[string]string) (domain.Conversation, error) {
	threadID, err := parseConversationInt64(row, "_id")
	if err != nil {
		return domain.Conversation{}, err
	}
	unread, err := parseConversationInt64(row, "unread_count")
	if err != nil || unread < 0 {
		return domain.Conversation{}, fmt.Errorf("invalid conversation unread_count")
	}
	date, err := parseConversationInt64(row, "date")
	if err != nil {
		return domain.Conversation{}, err
	}
	recipientIDs, err := parseRecipientIDs(row["recipient_ids"])
	if err != nil {
		return domain.Conversation{}, err
	}

	participants := make([]domain.Contact, 0, len(recipientIDs))
	titles := make([]string, 0, len(recipientIDs))
	for _, recipientID := range recipientIDs {
		address, ok := canonical[recipientID]
		if !ok || address == "" {
			participants = append(participants, domain.Contact{ID: recipientID, DisplayName: "Unknown participant"})
			titles = append(titles, "Unknown participant")
			continue
		}
		title := address
		if name := contacts[domain.NormalizePhone(address)]; name != "" {
			title = name
		}
		participants = append(participants, domain.Contact{ID: recipientID, DisplayName: title, Phone: address})
		titles = append(titles, title)
	}
	if len(titles) == 0 {
		titles = append(titles, "Unknown participant")
	}
	return domain.Conversation{
		ThreadID:     threadID,
		Title:        strings.Join(titles, ", "),
		Participants: participants,
		Snippet:      row["snippet"],
		UpdatedAt:    time.UnixMilli(date),
		UnreadCount:  int(unread),
	}, nil
}

func parseRecipientIDs(value string) ([]int64, error) {
	fields := strings.Fields(value)
	ids := make([]int64, 0, len(fields))
	for _, field := range fields {
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%w: malformed recipient_ids", ErrProviderOutput)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseConversationInt64(row map[string]string, field string) (int64, error) {
	value, ok := row[field]
	if !ok {
		return 0, fmt.Errorf("%w: missing conversation field %s", ErrProviderOutput, field)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid conversation field %s", ErrProviderOutput, field)
	}
	return parsed, nil
}
