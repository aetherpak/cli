package signing

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/aetherpak/aetherpak/pkg/logger"
)

// Signer handles in-memory GPG detached signature generation.
type Signer struct {
	entityList openpgp.EntityList
}

// NewSigner creates a new Signer from armored or binary GPG private key strings.
// A non-empty passphrase unlocks keys whose secret material is encrypted.
func NewSigner(privateKeys []string, passphrase string) (*Signer, error) {
	logger.Debug("Loading %d private GPG key(s) into memory.", len(privateKeys))

	var mergedList openpgp.EntityList
	for i, key := range privateKeys {
		if key == "" {
			continue
		}
		reader := bytes.NewBufferString(key)
		el, err := openpgp.ReadArmoredKeyRing(reader)
		if err != nil {
			// Attempt to read as raw binary keyring
			reader = bytes.NewBufferString(key)
			el, err = openpgp.ReadKeyRing(reader)
			if err != nil {
				return nil, fmt.Errorf("failed to parse GPG private key at index %d: %w", i, err)
			}
		}
		mergedList = append(mergedList, el...)
	}

	if len(mergedList) == 0 {
		return nil, fmt.Errorf("no GPG keys loaded")
	}

	if passphrase != "" {
		for i, entity := range mergedList {
			if err := decryptEntity(entity, []byte(passphrase)); err != nil {
				return nil, fmt.Errorf("failed to unlock GPG key at index %d: %w", i, err)
			}
		}
	}

	return &Signer{entityList: mergedList}, nil
}

// decryptEntity unlocks an entity's encrypted primary and subkey secret material.
func decryptEntity(entity *openpgp.Entity, passphrase []byte) error {
	if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
		if err := entity.PrivateKey.Decrypt(passphrase); err != nil {
			return err
		}
	}
	for _, sub := range entity.Subkeys {
		if sub.PrivateKey != nil && sub.PrivateKey.Encrypted {
			if err := sub.PrivateKey.Decrypt(passphrase); err != nil {
				return err
			}
		}
	}
	return nil
}

// Sign generates a single GPG signed message using the first key.
func (s *Signer) Sign(r io.Reader) ([]byte, error) {
	var sig bytes.Buffer

	if len(s.entityList) == 0 {
		return nil, fmt.Errorf("no key entities available for signing")
	}

	w, err := openpgp.Sign(&sig, s.entityList[0], nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sign wrapper: %w", err)
	}
	if _, err := io.Copy(w, r); err != nil {
		return nil, fmt.Errorf("failed to write payload: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize sign wrapper: %w", err)
	}

	return sig.Bytes(), nil
}

// SignWithAll signs the data bytes with all loaded keys, returning a slice of GPG signed messages.
func (s *Signer) SignWithAll(data []byte) ([][]byte, error) {
	var signatures [][]byte
	for i, entity := range s.entityList {
		var sig bytes.Buffer
		w, err := openpgp.Sign(&sig, entity, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize sign wrapper for key at index %d: %w", i, err)
		}
		if _, err := w.Write(data); err != nil {
			return nil, fmt.Errorf("failed to write payload for key at index %d: %w", i, err)
		}
		if err := w.Close(); err != nil {
			return nil, fmt.Errorf("failed to finalize sign wrapper for key at index %d: %w", i, err)
		}
		signatures = append(signatures, sig.Bytes())
	}
	return signatures, nil
}

// ExportArmoredPublicKeyRing exports the armored, concatenated GPG public keyring for all entities.
func (s *Signer) ExportArmoredPublicKeyRing() (string, error) {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", fmt.Errorf("failed to initialize armor encoder: %w", err)
	}
	for i, entity := range s.entityList {
		if err := entity.Serialize(w); err != nil {
			w.Close()
			return "", fmt.Errorf("failed to serialize public keyring entity at index %d: %w", i, err)
		}
	}
	w.Close()
	return buf.String(), nil
}

// ExportBase64PublicKeyRing exports the base64-encoded binary public keyring.
func (s *Signer) ExportBase64PublicKeyRing() (string, error) {
	var buf bytes.Buffer
	for i, entity := range s.entityList {
		if err := entity.Serialize(&buf); err != nil {
			return "", fmt.Errorf("failed to serialize public keyring entity at index %d: %w", i, err)
		}
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Fingerprint returns the 40-character uppercase hexadecimal GPG fingerprint of the first loaded key.
func (s *Signer) Fingerprint() string {
	if len(s.entityList) == 0 {
		return ""
	}
	return fmt.Sprintf("%X", s.entityList[0].PrimaryKey.Fingerprint)
}
