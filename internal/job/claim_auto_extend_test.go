package job

import (
	"testing"
	"time"
)

// P7 — When an actor who currently holds a claim performs a "still
// working" write (note, edit, label add/remove) on the claimed task,
// the claim's TTL should reset to now + DefaultClaimTTLSeconds. This
// removes the need to call `job heartbeat` explicitly for the common
// case where the agent is actively writing.

func TestAutoExtendClaim_Note_ExtendsTTLWhenHolderWrites(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	base := time.Now()
	CurrentNowFunc = func() time.Time { return base }
	MustClaim(t, db, id, "10m") // shorter than the default so auto-extend actually extends

	// 2 minutes later, the holder notes the task.
	CurrentNowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	if err := RunNote(db, id, "still working", nil, TestActor); err != nil {
		t.Fatalf("note: %v", err)
	}

	task := MustGet(t, db, id)
	if task.ClaimExpiresAt == nil {
		t.Fatal("claim_expires_at should still be set")
	}
	want := base.Add(2*time.Minute).Unix() + DefaultClaimTTLSeconds
	if *task.ClaimExpiresAt != want {
		t.Errorf("claim_expires_at: got %d, want %d (now + DefaultTTL)", *task.ClaimExpiresAt, want)
	}
}

func TestAutoExtendClaim_Edit_ExtendsTTLWhenHolderWrites(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	base := time.Now()
	CurrentNowFunc = func() time.Time { return base }
	MustClaim(t, db, id, "10m")

	CurrentNowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	newTitle := "T (edited)"
	if err := RunEdit(db, id, &newTitle, nil, TestActor); err != nil {
		t.Fatalf("edit: %v", err)
	}

	task := MustGet(t, db, id)
	want := base.Add(2*time.Minute).Unix() + DefaultClaimTTLSeconds
	if task.ClaimExpiresAt == nil || *task.ClaimExpiresAt != want {
		got := int64(0)
		if task.ClaimExpiresAt != nil {
			got = *task.ClaimExpiresAt
		}
		t.Errorf("edit did not auto-extend: got %d, want %d", got, want)
	}
}

func TestAutoExtendClaim_LabelAdd_ExtendsTTLWhenHolderWrites(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	base := time.Now()
	CurrentNowFunc = func() time.Time { return base }
	MustClaim(t, db, id, "10m")

	CurrentNowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := RunLabelAdd(db, id, []string{"wip"}, TestActor); err != nil {
		t.Fatalf("label add: %v", err)
	}

	task := MustGet(t, db, id)
	want := base.Add(2*time.Minute).Unix() + DefaultClaimTTLSeconds
	if task.ClaimExpiresAt == nil || *task.ClaimExpiresAt != want {
		got := int64(0)
		if task.ClaimExpiresAt != nil {
			got = *task.ClaimExpiresAt
		}
		t.Errorf("label add did not auto-extend: got %d, want %d", got, want)
	}
}

func TestAutoExtendClaim_LabelRemove_ExtendsTTLWhenHolderWrites(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	base := time.Now()
	CurrentNowFunc = func() time.Time { return base }
	MustClaim(t, db, id, "10m")
	if _, err := RunLabelAdd(db, id, []string{"wip"}, TestActor); err != nil {
		t.Fatalf("seed label: %v", err)
	}

	CurrentNowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := RunLabelRemove(db, id, []string{"wip"}, TestActor); err != nil {
		t.Fatalf("label remove: %v", err)
	}

	task := MustGet(t, db, id)
	want := base.Add(2*time.Minute).Unix() + DefaultClaimTTLSeconds
	if task.ClaimExpiresAt == nil || *task.ClaimExpiresAt != want {
		got := int64(0)
		if task.ClaimExpiresAt != nil {
			got = *task.ClaimExpiresAt
		}
		t.Errorf("label remove did not auto-extend: got %d, want %d", got, want)
	}
}

// Extend should NEVER shorten. If the actor deliberately claimed for a
// long duration (e.g., 4h), a later note 10 minutes in must NOT drop
// the expiry back to now + 30m.
func TestAutoExtendClaim_DoesNotShorten_WhenCurrentExpiryIsFurtherOut(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	base := time.Now()
	CurrentNowFunc = func() time.Time { return base }
	MustClaim(t, db, id, "4h")
	longExpiry := base.Unix() + 4*3600

	CurrentNowFunc = func() time.Time { return base.Add(10 * time.Minute) }
	if err := RunNote(db, id, "checkpoint", nil, TestActor); err != nil {
		t.Fatalf("note: %v", err)
	}

	task := MustGet(t, db, id)
	if task.ClaimExpiresAt == nil || *task.ClaimExpiresAt != longExpiry {
		got := int64(0)
		if task.ClaimExpiresAt != nil {
			got = *task.ClaimExpiresAt
		}
		t.Errorf("auto-extend must not shorten a longer explicit expiry: got %d, want %d",
			got, longExpiry)
	}
}

// A note on a task held by SOMEONE ELSE must be rejected by the
// existing claim-ownership check; auto-extend must not silently
// override the other actor's lock. (Regression guard: make sure the
// P7 changes don't loosen ownership enforcement.)
func TestAutoExtendClaim_NoteByNonHolder_StillRejected(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")
	MustClaim(t, db, id, "1h") // held by TestActor

	err := RunNote(db, id, "meddling", nil, "other-actor")
	if err == nil {
		t.Fatal("note by non-holder on claimed task must error")
	}
}

// Label add/remove currently do NOT enforce claim ownership (non-holders
// may label a claimed task). P7 must not extend the claim in that case,
// or another agent could quietly push a held claim forward.
func TestAutoExtendClaim_LabelByNonHolder_DoesNotExtendHoldersClaim(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	base := time.Now()
	CurrentNowFunc = func() time.Time { return base }
	MustClaim(t, db, id, "10m")
	beforeExpiry := base.Unix() + 10*60

	CurrentNowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := RunLabelAdd(db, id, []string{"from-other"}, "other-actor"); err != nil {
		t.Fatalf("label add by non-holder: %v", err)
	}

	task := MustGet(t, db, id)
	if task.ClaimExpiresAt == nil || *task.ClaimExpiresAt != beforeExpiry {
		got := int64(0)
		if task.ClaimExpiresAt != nil {
			got = *task.ClaimExpiresAt
		}
		t.Errorf("non-holder's label write must not alter holder's claim expiry: got %d, want %d",
			got, beforeExpiry)
	}
}

// A write to a task that is NOT currently claimed must leave the claim
// fields as-is (both nil).
func TestAutoExtendClaim_NoteOnUnclaimedTask_NoClaimFieldsSet(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "T")

	if err := RunNote(db, id, "drive-by", nil, TestActor); err != nil {
		t.Fatalf("note: %v", err)
	}

	task := MustGet(t, db, id)
	if task.ClaimExpiresAt != nil {
		t.Errorf("claim_expires_at should remain nil on unclaimed task: got %v", *task.ClaimExpiresAt)
	}
	if task.ClaimedBy != nil {
		t.Errorf("claimed_by should remain nil on unclaimed task: got %v", *task.ClaimedBy)
	}
}
