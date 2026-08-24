//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/protocol"
	"github.com/galaxytty/galaxytty/internal/remote"
)

func TestRealHelperReplyTargetReadOnly(t *testing.T) {
	if os.Getenv("GALAXYTTY_REAL_DEVICE_TEST") != "1" {
		t.Skip("explicit read-only Helper device test opt-in required")
	}
	target := requirePrivateTestValue(t, "GALAXYTTY_TEST_RECIPIENT")
	client := connectRealHelper(t)
	conversation := resolveReplyTarget(t, client, target)
	requireReplyCapability(t, client, conversation.ThreadID)
	t.Log("the authorized target resolves to exactly one active RemoteInput reply action")
}

func TestRealHelperReplySend(t *testing.T) {
	if os.Getenv("GALAXYTTY_REAL_DEVICE_TEST") != "1" || os.Getenv("GALAXYTTY_ENABLE_SEND_TEST") != "1" {
		t.Skip("explicit real-send Helper device test opt-in required")
	}
	target := requirePrivateTestValue(t, "GALAXYTTY_TEST_RECIPIENT")
	text := requirePrivateTestValue(t, "GALAXYTTY_TEST_TEXT")
	client := connectRealHelper(t)
	conversation := resolveReplyTarget(t, client, target)
	requireReplyCapability(t, client, conversation.ThreadID)

	_, err := remote.NewSender(client).SendToConversation(context.Background(), conversation.ThreadID, text)
	if !errors.Is(err, remote.ErrSendEvidenceUnavailable) {
		t.Fatalf("RemoteInput action was not accepted as unverified: %v", err)
	}
	t.Log("RemoteInput action accepted; outgoing delivery remains unverified")
}

func TestRealHelperOutgoingProviderEvidenceReadOnly(t *testing.T) {
	if os.Getenv("GALAXYTTY_REAL_DEVICE_TEST") != "1" {
		t.Skip("explicit read-only Helper device test opt-in required")
	}
	text := requirePrivateTestValue(t, "GALAXYTTY_TEST_TEXT")
	notBefore, err := strconv.ParseInt(requirePrivateTestValue(t, "GALAXYTTY_SEND_NOT_BEFORE_MILLIS"), 10, 64)
	if err != nil || notBefore <= 0 {
		t.Fatal("GALAXYTTY_SEND_NOT_BEFORE_MILLIS must be a positive Unix millisecond timestamp")
	}
	client := connectRealHelper(t)
	messages, err := remote.NewStore(client).MessagesAfter(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var matches int
	for _, message := range messages {
		if message.Direction == domain.DirectionOutgoing &&
			message.Body == text &&
			message.Timestamp.UnixMilli() >= notBefore {
			matches++
		}
	}
	t.Logf("exact outgoing SMS Provider evidence count=%d", matches)
}

func connectRealHelper(t *testing.T) *remote.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	credentials, err := pairing.DefaultStore()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	client, err := remote.NewDiscoveredClient(
		remote.Config{Credentials: credentials},
		remote.NewDiscovery(4*time.Second),
	)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	runContext, stop := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(runContext) }()
	t.Cleanup(func() {
		stop()
		cancel()
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("Helper client shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Helper client did not stop")
		}
	})
	if err := client.WaitConnected(ctx); err != nil {
		t.Fatal(err)
	}
	return client
}

func resolveReplyTarget(t *testing.T, client *remote.Client, target string) domain.Conversation {
	t.Helper()
	store := remote.NewStore(client)
	conversations, err := store.Conversations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	messages, err := store.MessagesAfter(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	matches := matchExactTarget(conversations, messages, target)
	if len(matches) > 0 {
		if len(matches) != 1 {
			t.Fatalf("exact target conversation match count=%d", len(matches))
		}
		requireReplyCapability(t, client, matches[0].ThreadID)
		return matches[0]
	}
	if os.Getenv("GALAXYTTY_ALLOW_SOLE_ACTIVE_REPLY") != "1" {
		t.Fatal("target label was not exported verbatim; GALAXYTTY_ALLOW_SOLE_ACTIVE_REPLY=1 is required")
	}

	active := make([]domain.Conversation, 0, 1)
	for _, conversation := range conversations {
		available, err := replyCapability(client, conversation.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		if available {
			active = append(active, conversation)
		}
	}
	if len(active) != 1 {
		t.Fatalf("active RemoteInput target count=%d after exact label match count=0", len(active))
	}
	t.Log("target label was not exported verbatim; using the sole active Samsung notification reply action")
	return active[0]
}

func matchExactTarget(conversations []domain.Conversation, messages []domain.Message, target string) []domain.Conversation {
	target = strings.TrimSpace(target)
	threadMatches := make(map[int64]struct{})
	for _, conversation := range conversations {
		if strings.TrimSpace(conversation.Title) == target {
			threadMatches[conversation.ThreadID] = struct{}{}
			continue
		}
		for _, participant := range conversation.Participants {
			if strings.TrimSpace(participant.DisplayName) == target {
				threadMatches[conversation.ThreadID] = struct{}{}
				break
			}
		}
	}
	for _, message := range messages {
		if strings.TrimSpace(message.Address) == target {
			threadMatches[message.ThreadID] = struct{}{}
		}
	}
	matches := make([]domain.Conversation, 0, len(threadMatches))
	for _, conversation := range conversations {
		if _, found := threadMatches[conversation.ThreadID]; found {
			matches = append(matches, conversation)
		}
	}
	return matches
}

func TestMatchExactTargetRejectsAmbiguousAndPartialNames(t *testing.T) {
	conversations := []domain.Conversation{
		{ThreadID: 1, Title: "테스트 대상 가족"},
		{ThreadID: 2, Title: "Samsung Messages"},
		{ThreadID: 3, Participants: []domain.Contact{{DisplayName: "테스트 대상"}}},
	}
	messages := []domain.Message{
		{ThreadID: 1, Address: "테스트 대상"},
		{ThreadID: 2, Address: "다른 사람"},
	}
	if matches := matchExactTarget(conversations, messages, "테스트 대상 가족"); len(matches) != 1 || matches[0].ThreadID != 1 {
		t.Fatalf("title matches=%+v", matches)
	}
	if matches := matchExactTarget(conversations, messages, "테스트"); len(matches) != 0 {
		t.Fatalf("partial matches=%+v", matches)
	}
	if matches := matchExactTarget(conversations, messages, "테스트 대상"); len(matches) != 2 {
		t.Fatalf("ambiguous matches=%+v", matches)
	}
}

func requireReplyCapability(t *testing.T, client *remote.Client, threadID int64) {
	t.Helper()
	available, err := replyCapability(client, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Fatal("target conversation has no active RemoteInput reply action")
	}
}

func replyCapability(client *remote.Client, threadID int64) (bool, error) {
	response, err := client.Request(context.Background(), protocol.TypeGetReplyCapability, map[string]any{
		"threadId": threadID,
	})
	if err != nil {
		return false, err
	}
	if response.Type != protocol.TypeReplyCapability {
		return false, errors.New("unexpected reply capability response")
	}
	var payload struct {
		Available bool `json:"available"`
	}
	if err := response.DecodePayload(&payload); err != nil {
		return false, err
	}
	return payload.Available, nil
}

func requirePrivateTestValue(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
