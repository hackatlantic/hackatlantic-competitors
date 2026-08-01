package entitlements

import "testing"

func TestResolvePrefersAttendeeOverrideOverCheckpointDefaults(t *testing.T) {
	defaultRule := Resolve(true, 3, nil)
	if !defaultRule.Allowed || defaultRule.MaxRedemptions != 3 {
		t.Fatalf("unexpected checkpoint default rule: %+v", defaultRule)
	}

	deniedOverride := Resolve(true, 3, &Override{Allowed: false, MaxRedemptions: 99})
	if deniedOverride.Allowed || deniedOverride.MaxRedemptions != 99 {
		t.Fatalf("denied attendee override did not replace the default: %+v", deniedOverride)
	}

	allowedOverride := Resolve(false, 0, &Override{Allowed: true, MaxRedemptions: 1})
	if !allowedOverride.Allowed || allowedOverride.MaxRedemptions != 1 {
		t.Fatalf("allowed attendee override did not replace the default: %+v", allowedOverride)
	}
}
