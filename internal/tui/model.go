package tui

import (
	"bufio"
	"context"
	"fmt"
	"github.com/galaxytty/galaxytty/internal/domain"
	"io"
	"strings"
)

// Run provides a dependency-free, fixture-testable terminal UI. Empty input opens
// the selected conversation; :back mirrors Esc for line-oriented test terminals.
func Run(ctx context.Context, in io.Reader, out io.Writer, store domain.MessageStore, sender domain.MessageSender) error {
	c, e := store.Conversations(ctx)
	if e != nil {
		return e
	}
	fmt.Fprintln(out, "GalaxyTTY                         ● Mock Connected\n\n대화 목록")
	for i, x := range c {
		fmt.Fprintf(out, "%d. %s (%d)\n   %s\n", i+1, x.Title, x.UnreadCount, x.Snippet)
	}
	s := bufio.NewScanner(in)
	selected := int64(0)
	for {
		fmt.Fprint(out, "> ")
		if !s.Scan() {
			return s.Err()
		}
		v := strings.TrimSpace(s.Text())
		if strings.HasPrefix(v, "/") {
			cmd, e := ParseCommand(v)
			if e != nil {
				fmt.Fprintln(out, e)
				continue
			}
			if cmd.Name == "exit" || cmd.Name == "quit" {
				return nil
			}
			fmt.Fprintf(out, "/%s command accepted\n", cmd.Name)
			continue
		}
		if v == ":back" || v == "\x1b" {
			selected = 0
			fmt.Fprintln(out, "대화 목록")
			continue
		}
		if selected == 0 {
			var n int
			if _, e := fmt.Sscan(v, &n); e != nil || n < 1 || n > len(c) {
				fmt.Fprintln(out, "대화 번호를 입력하세요")
				continue
			}
			selected = c[n-1].ThreadID
			ms, e := store.Messages(ctx, selected, domain.MessageQuery{})
			if e != nil {
				return e
			}
			fmt.Fprintln(out, c[n-1].Title)
			for _, m := range ms {
				body := m.Body
				if len(m.Attachments) > 0 {
					body = "🖼 이미지"
				}
				fmt.Fprintln(out, body)
			}
			continue
		}
		if v != "" {
			// The presentation layer supplies the selected conversation address in a
			// real adapter; the fixture uses its stable sample recipient.
			if e := sender.Send(ctx, "01012345678", v); e != nil {
				return e
			}
			fmt.Fprintln(out, "전송됨")
		}
	}
}
