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
