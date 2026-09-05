package daemon

import "testing"

func TestClientLeaseAllowsOnlyOneInteractiveOwner(t *testing.T) {
	var l clientLease
	id1, perr := l.acquire()
	if perr != nil || id1 == "" {
		t.Fatalf("first acquire: id=%q err=%v", id1, perr)
	}
	if _, perr := l.acquire(); perr == nil || perr.Code != "ipc.client_busy" {
		t.Fatalf("expected client_busy, got %v", perr)
	}
	if !l.release(id1) {
		t.Fatal("expected owner release to succeed")
	}
	id2, perr := l.acquire()
	if perr != nil || id2 == "" || id2 == id1 {
		t.Fatalf("reacquire: id=%q err=%v", id2, perr)
	}
}

func TestClientLeaseIgnoresNonOwnerRelease(t *testing.T) {
	var l clientLease
	id, perr := l.acquire()
	if perr != nil {
		t.Fatal(perr)
	}
	if l.release("other") {
		t.Fatal("non-owner release succeeded")
	}
	if _, perr := l.acquire(); perr == nil || perr.Code != "ipc.client_busy" {
		t.Fatalf("lease should still be active, got %v", perr)
	}
	if !l.release(id) {
		t.Fatal("owner release failed")
	}
}
