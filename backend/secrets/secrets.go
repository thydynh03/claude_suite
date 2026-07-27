// Package secrets seals small credential files to the local Windows user.
//
// anti_accounts.json holds Google refresh tokens. 0600 permissions keep other
// accounts out, but anything running AS the user — or a copied disk, a synced
// backup, a pasted "can you look at my data dir" — reads them in plain text.
// DPAPI (CryptProtectData) encrypts with a key derived from the user's logon
// credentials: no passphrase to manage, nothing stored beside the file, and
// the blob is useless off this machine and account.
//
// On non-Windows builds sealing is the identity function. That is a
// documented trade, not an oversight: the app ships for Windows, the other
// platforms exist for CI, and inventing a keyring story for them would be
// untested code guarding nothing.
package secrets

import (
	"bytes"
	"fmt"
)

// header marks a sealed file. Legacy files start with JSON ('[' or '{'), so
// its absence is what makes transparent migration possible.
const header = "agent-center-sealed-v1\n"
const legacyHeader = "claude-suite-sealed-v1\n"

// Seal wraps plaintext for storage.
func Seal(plaintext []byte) ([]byte, error) {
	sealed, err := protect(plaintext)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	return append([]byte(header), sealed...), nil
}

// Open returns the plaintext of data whether it is sealed or legacy plain
// JSON, and reports which it was — a caller seeing wasPlain=true should
// re-save, which is how existing installs migrate without a step.
func Open(data []byte) (plaintext []byte, wasPlain bool, err error) {
	if bytes.HasPrefix(data, []byte(header)) {
		plaintext, err = unprotect(bytes.TrimPrefix(data, []byte(header)))
		if err != nil {
			return nil, false, fmt.Errorf("open sealed data (wrong machine or user account?): %w", err)
		}
		return plaintext, false, nil
	}
	if bytes.HasPrefix(data, []byte(legacyHeader)) {
		plaintext, err = unprotect(bytes.TrimPrefix(data, []byte(legacyHeader)))
		if err != nil {
			return nil, false, fmt.Errorf("open legacy sealed data (wrong machine or user account?): %w", err)
		}
		return plaintext, true, nil
	}
	return data, true, nil
}
