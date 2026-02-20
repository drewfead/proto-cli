package cliauth

import (
	"context"
	"errors"

	"github.com/zalando/go-keyring"
)

const keychainAccount = "default"

// KeychainStore persists credentials using the OS keychain
// (macOS Keychain, Windows Credential Manager, Linux Secret Service).
type KeychainStore struct {
	serviceName string
}

// NewKeychainStore creates a KeychainStore that stores credentials under the
// given application name as the keychain service name.
func NewKeychainStore(appName string) *KeychainStore {
	return &KeychainStore{serviceName: appName}
}

// Save persists a credential token to the keychain.
func (s *KeychainStore) Save(_ context.Context, token []byte) error {
	return keyring.Set(s.serviceName, keychainAccount, string(token))
}

// Load retrieves the stored credential token from the keychain.
// Returns ErrNotFound if no credential is stored.
func (s *KeychainStore) Load(_ context.Context) ([]byte, error) {
	secret, err := keyring.Get(s.serviceName, keychainAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return []byte(secret), nil
}

// Delete removes the stored credential from the keychain.
// Returns ErrNotFound if no credential is stored.
func (s *KeychainStore) Delete(_ context.Context) error {
	err := keyring.Delete(s.serviceName, keychainAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
