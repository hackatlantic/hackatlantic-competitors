// Package entitlements owns effective checkpoint access rules.
package entitlements

// Override is the persisted attendee-specific rule, when one exists.
type Override struct {
	Allowed        bool
	MaxRedemptions int32
}

// Effective is the rule that governs a single attendee at a checkpoint.
type Effective struct {
	Allowed        bool
	MaxRedemptions int32
}

// Resolve applies the required precedence: an attendee-specific entitlement
// replaces both checkpoint defaults. The database rejects negative maxima; the
// defensive clamp keeps callers safe if a malformed fixture bypasses it.
func Resolve(defaultAllowed bool, defaultMaxRedemptions int32, override *Override) Effective {
	result := Effective{Allowed: defaultAllowed, MaxRedemptions: defaultMaxRedemptions}
	if override != nil {
		result = Effective{Allowed: override.Allowed, MaxRedemptions: override.MaxRedemptions}
	}
	if result.MaxRedemptions < 0 {
		result.MaxRedemptions = 0
	}
	return result
}
