package firestoredb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// TestListUsers_PaginationAndTotalCount verifies that ListUsers returns
// the correct total count and paginated results, regardless of page size.
// This is a regression test for the count fallback bug fixed in PR #456:
// when Firestore's aggregation count fails, the fallback must still
// return the true total — not len(snaps) capped by the page limit.
func TestListUsers_PaginationAndTotalCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	const numUsers = 5

	// Create test users.
	userIDs := make([]string, numUsers)
	for i := range numUsers {
		id := fmt.Sprintf("list-test-%d-%d", time.Now().UnixNano(), i)
		userIDs[i] = id
		err := s.CreateUser(ctx, &storage.UserRecord{
			ID:     id,
			Email:  fmt.Sprintf("user%d@test.com", i),
			Status: "active",
			Role:   "developer",
		})
		if err != nil {
			t.Fatalf("CreateUser[%d]: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range userIDs {
			if id != "" {
				cleanupUser(ctx, s, id)
			}
		}
	})

	// ── Test 1: full page (limit >= numUsers) ──
	users, total, err := s.ListUsers(ctx, "", numUsers+10, 0)
	if err != nil {
		t.Fatalf("ListUsers full page: %v", err)
	}
	if total < numUsers {
		t.Errorf("total = %d, want >= %d", total, numUsers)
	}
	if len(users) < numUsers {
		t.Errorf("got %d users, want >= %d", len(users), numUsers)
	}

	// ── Test 2: paginated (limit < total) ──
	// Page 1: limit=2, offset=0
	page1, total1, err := s.ListUsers(ctx, "", 2, 0)
	if err != nil {
		t.Fatalf("ListUsers page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page 1: got %d users, want 2", len(page1))
	}
	if total1 < numUsers {
		t.Errorf("page 1 total = %d, want >= %d (must not be capped by page limit)", total1, numUsers)
	}

	// Page 2: limit=2, offset=2
	page2, total2, err := s.ListUsers(ctx, "", 2, 2)
	if err != nil {
		t.Fatalf("ListUsers page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page 2: got %d users, want 2", len(page2))
	}
	if total2 != total1 {
		t.Errorf("total changed between pages: page1=%d, page2=%d", total1, total2)
	}

	// Page 3 (last page): limit=2, offset=4
	page3, total3, err := s.ListUsers(ctx, "", 2, 4)
	if err != nil {
		t.Fatalf("ListUsers page 3: %v", err)
	}
	if len(page3) < 1 {
		t.Errorf("page 3: got %d users, want >= 1", len(page3))
	}
	if total3 != total1 {
		t.Errorf("total changed between pages: page1=%d, page3=%d", total1, total3)
	}

	// ── Test 3: past-last-page (offset > total) ──
	// Use the observed total to derive a guaranteed past-end offset,
	// in case the emulator has pre-existing rows.
	pastEnd, totalPast, err := s.ListUsers(ctx, "", 2, total1)
	if err != nil {
		t.Fatalf("ListUsers past end: %v", err)
	}
	if len(pastEnd) != 0 {
		t.Errorf("past end: got %d users, want 0", len(pastEnd))
	}
	if totalPast < numUsers {
		t.Errorf("past end total = %d, want >= %d (must report true count even for empty pages)", totalPast, numUsers)
	}

	// ── Test 4: status filter ──
	filteredUsers, filteredTotal, err := s.ListUsers(ctx, "active", 100, 0)
	if err != nil {
		t.Fatalf("ListUsers with status filter: %v", err)
	}
	if filteredTotal < numUsers {
		t.Errorf("filtered total = %d, want >= %d", filteredTotal, numUsers)
	}
	if len(filteredUsers) < numUsers {
		t.Errorf("filtered users = %d, want >= %d", len(filteredUsers), numUsers)
	}
}

// TestListUsers_TotalConsistentAcrossPages verifies that the total count
// remains the same regardless of which page is requested. This catches
// the pre-#456 bug where total = len(snaps) = min(limit, actual_count).
func TestListUsers_TotalConsistentAcrossPages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	const numUsers = 7
	const pageSize = 3

	userIDs := make([]string, numUsers)
	for i := range numUsers {
		id := fmt.Sprintf("consistent-%d-%d", time.Now().UnixNano(), i)
		userIDs[i] = id
		err := s.CreateUser(ctx, &storage.UserRecord{
			ID:     id,
			Email:  fmt.Sprintf("consist%d@test.com", i),
			Status: "active",
			Role:   "developer",
		})
		if err != nil {
			t.Fatalf("CreateUser[%d]: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range userIDs {
			if id != "" {
				cleanupUser(ctx, s, id)
			}
		}
	})

	// Fetch every page and verify total is consistent.
	var prevTotal int
	for offset := 0; offset < numUsers+pageSize; offset += pageSize {
		_, total, err := s.ListUsers(ctx, "", pageSize, offset)
		if err != nil {
			t.Fatalf("ListUsers offset=%d: %v", offset, err)
		}
		if total < numUsers {
			t.Errorf("offset=%d: total=%d, want >= %d", offset, total, numUsers)
		}
		if prevTotal != 0 && total != prevTotal {
			t.Errorf("total changed: offset=%d got %d, previous was %d", offset, total, prevTotal)
		}
		prevTotal = total
	}
}
