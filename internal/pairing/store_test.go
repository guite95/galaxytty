package pairing

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStorePersistsCredentialPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")
	store := NewFileStore(path)
	want := Credential{DeviceID: "device-1", DeviceName: "Galaxy Test", ClientID: "client-1", Secret: make([]byte, CredentialSize)}
	for index := range want.Secret {
		want.Secret[index] = byte(index)
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	got, err := store.Load("device-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceName != want.DeviceName || got.ClientID != want.ClientID || FormatCode(got.Secret) != FormatCode(want.Secret) {
		t.Fatalf("credential=%+v", got)
	}
}

func TestFileStoreReportsUnpairedDevice(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "credentials.json"))
	if _, err := store.Load("missing"); !errors.Is(err, ErrNotPaired) {
		t.Fatalf("err=%v", err)
	}
}

func TestFileStoreRejectsMalformedCredentialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(path).Load("device-1"); err == nil {
		t.Fatal("expected malformed credential file error")
	}
}

func TestFileStoreRejectsCredentialFileReadableByOtherUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"devices":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(path).Load("device-1"); err == nil {
		t.Fatal("expected insecure credential file error")
	}
}
