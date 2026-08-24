package pairing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrNotPaired = errors.New("Galaxy Helper pairing is required")

type Credential struct {
	DeviceID   string
	DeviceName string
	ClientID   string
	Secret     []byte
}

type Store interface {
	Load(deviceID string) (Credential, error)
	Save(Credential) error
}

type FileStore struct {
	path string
}

type diskCredential struct {
	DeviceName string `json:"deviceName"`
	ClientID   string `json:"clientId"`
	Secret     string `json:"secret"`
}

type diskCredentials struct {
	Version int                       `json:"version"`
	Devices map[string]diskCredential `json:"devices"`
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func DefaultPath() (string, error) {
	if root := os.Getenv("XDG_CONFIG_HOME"); root != "" {
		return filepath.Join(root, "galaxytty", "credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "galaxytty", "credentials.json"), nil
}

func DefaultStore() (*FileStore, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return NewFileStore(path), nil
}

func (store *FileStore) Load(deviceID string) (Credential, error) {
	contents, err := store.read()
	if err != nil {
		return Credential{}, err
	}
	stored, ok := contents.Devices[deviceID]
	if !ok {
		return Credential{}, ErrNotPaired
	}
	secret, err := codeEncoding.DecodeString(stored.Secret)
	if err != nil || len(secret) != CredentialSize || stored.ClientID == "" {
		return Credential{}, errors.New("stored Galaxy Helper credential is invalid")
	}
	return Credential{DeviceID: deviceID, DeviceName: stored.DeviceName, ClientID: stored.ClientID, Secret: secret}, nil
}

func (store *FileStore) Save(credential Credential) error {
	if credential.DeviceID == "" || credential.ClientID == "" || len(credential.Secret) != CredentialSize {
		return errors.New("cannot save invalid Galaxy Helper credential")
	}
	contents, err := store.read()
	if err != nil {
		return err
	}
	contents.Devices[credential.DeviceID] = diskCredential{
		DeviceName: credential.DeviceName,
		ClientID:   credential.ClientID,
		Secret:     codeEncoding.EncodeToString(credential.Secret),
	}
	encoded, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Galaxy Helper credentials: %w", err)
	}
	directory := filepath.Dir(store.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create GalaxyTTY config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".credentials-*")
	if err != nil {
		return fmt.Errorf("create temporary credential file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary credential file: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary credential file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary credential file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary credential file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("replace Galaxy Helper credential file: %w", err)
	}
	return nil
}

func (store *FileStore) read() (diskCredentials, error) {
	contents := diskCredentials{Version: 1, Devices: make(map[string]diskCredential)}
	info, statErr := os.Stat(store.path)
	if statErr == nil && info.Mode().Perm()&0o077 != 0 {
		return contents, errors.New("Galaxy Helper credential file permissions must be 0600")
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return contents, fmt.Errorf("inspect Galaxy Helper credentials: %w", statErr)
	}
	encoded, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return contents, nil
	}
	if err != nil {
		return contents, fmt.Errorf("read Galaxy Helper credentials: %w", err)
	}
	if err := json.Unmarshal(encoded, &contents); err != nil {
		return contents, fmt.Errorf("decode Galaxy Helper credentials: %w", err)
	}
	if contents.Version != 1 || contents.Devices == nil {
		return contents, errors.New("unsupported Galaxy Helper credential file")
	}
	return contents, nil
}

func NewClientID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create GalaxyTTY client ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}
