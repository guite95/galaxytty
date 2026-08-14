package samsung

import (
	"context"
	"errors"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestSamsungMessagesDefaultRoleRequiresExactHolder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   bool
	}{
		{name: "Samsung", output: "com.samsung.android.messaging\n", want: true},
		{name: "multiple includes Samsung", output: "example.other\ncom.samsung.android.messaging\n", want: true},
		{name: "other", output: "example.other\n", want: false},
		{name: "prefix", output: "com.samsung.android.messaging.beta\n", want: false},
		{name: "empty", output: "\n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SamsungMessagesIsDefault(tc.output); got != tc.want {
				t.Fatalf("got=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestControllerRejectsMissingDefaultSMSRole(t *testing.T) {
	device := &controllerDevice{output: []byte("example.other\n")}
	controller, err := NewController(device, DefaultLayout())
	if err != nil {
		t.Fatal(err)
	}
	err = controller.EnsureDefaultSMSHandler(context.Background())
	if !errors.Is(err, domain.ErrSamsungMessagesNotDefault) {
		t.Fatalf("err=%v", err)
	}
}
