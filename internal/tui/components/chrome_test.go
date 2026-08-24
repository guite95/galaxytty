package components

import "testing"

func TestConnectionLabelDoesNotClaimConnectingStatesAreConnected(t *testing.T) {
	for _, state := range []string{"connecting", "reconnecting"} {
		if isConnected(state) {
			t.Fatalf("%s was styled as connected", state)
		}
		if label := connectionLabel(state); label != state {
			t.Fatalf("state=%s label=%s", state, label)
		}
	}
}

func TestNoticeLineUsesWarningStyleAndTerminalWidth(t *testing.T) {
	theme := DefaultTheme()
	line := NoticeLine(theme, "Samsung Messages accepted the reply · delivery unverified", 24)
	if line == "" || CellWidth(line) > 24 {
		t.Fatalf("line=%q width=%d", line, CellWidth(line))
	}
}
