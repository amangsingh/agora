// in agora/credential.go

package agora

import (
	"errors"
	"fmt"
)

// This file is the MECHANISM half of the SEC gate's authn boundary
// (advisory A). The framework offers credential establishment and
// verification as capabilities of the D bank, surfaced as methods on the
// Self beside the ResolveSelf choke point — and enforces nothing. POLICY
// (which requests must present a credential) belongs to the consumer at its
// membrane; pkg/server is the first one. A direct library consumer embedding
// a Self decides its own policy, which is why RunEpisode's auth posture is
// unchanged here.

// ErrSelfAuthentication is the single verification failure. It is
// deliberately one value for every failure shape — unknown self, wrong
// credential, credential never established — so no caller of the mechanism
// can build an existence oracle out of its errors.
var ErrSelfAuthentication = errors.New("self authentication failed")

// ErrSelfUnavailable is the establishment failure for an id that cannot be
// established (already taken, or lost to a concurrent winner). One value for
// both shapes: first contact is a race the loser survives, not a probe.
var ErrSelfUnavailable = errors.New("self unavailable")

// CredentialVerifier is the optional D-bank capability behind self
// verification. *storage.Repository implements it; in-memory test banks may.
// Implementations must return ErrSelfAuthentication for every failure shape
// and are expected to compare credentials in constant time.
type CredentialVerifier interface {
	VerifySelfCredential(selfID, credential string) error
}

// SelfEstablisher is the optional D-bank capability behind self
// establishment: create the self and mint its credential in one act, so a
// self cannot exist server-side without a credential to guard it.
// Implementations must be idempotence-safe under concurrent first contact
// (no failure mode that surfaces as an internal error; the loser receives
// ErrSelfUnavailable).
type SelfEstablisher interface {
	EstablishSelf(id, name string) (credential string, err error)
}

// EstablishSelf creates the addressed self through the D bank and returns
// the newly minted credential — the only time it is ever visible. An id
// that is already established (or lost to a concurrent establisher) yields
// ErrSelfUnavailable.
func (s *Self) EstablishSelf(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("cannot establish a self with an empty id")
	}
	if s.d == nil {
		return "", fmt.Errorf("self %q cannot be established: this Self has no memory bank (D)", id)
	}
	se, ok := s.d.(SelfEstablisher)
	if !ok {
		return "", fmt.Errorf("the D bank of self %q offers no establishment mechanism", s.id)
	}
	return se.EstablishSelf(id, id)
}

// VerifySelf checks a caller-presented credential for the addressed self
// through the D bank. It answers only "does this credential authenticate
// this self id" — it never resolves, creates, or recalls. A D bank without
// the verification capability fails closed.
func (s *Self) VerifySelf(selfID, credential string) error {
	if selfID == "" {
		return ErrSelfAuthentication
	}
	if s.d == nil {
		return fmt.Errorf("self %q cannot be verified: this Self has no memory bank (D)", selfID)
	}
	cv, ok := s.d.(CredentialVerifier)
	if !ok {
		return fmt.Errorf("the D bank of self %q offers no credential verification mechanism", s.id)
	}
	return cv.VerifySelfCredential(selfID, credential)
}
