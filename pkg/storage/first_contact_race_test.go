package storage

// SEC gate test (advisory C at the store): concurrent first contact for the
// same new self id must not fail on the primary key. On the pre-change tree
// EnsureSelf is get-then-create (membank.go), so simultaneous callers pass
// the read miss together and all but one lose the INSERT — this test is red
// there. The fix is an idempotent create; this test pins it.

import (
	"sync"
	"testing"
)

func TestEnsureSelf_ConcurrentFirstContactIdempotent(t *testing.T) {
	repo, err := NewRepository(t.TempDir() + "/race.db")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	const selfID = "self-race"
	const workers = 32

	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = repo.EnsureSelf(selfID)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("AC5 FAIL: concurrent first contact %d errored: %v (create must be idempotent)", i, err)
		}
	}

	if _, err := repo.GetSelf(selfID); err != nil {
		t.Fatalf("self %q missing after concurrent first contact: %v", selfID, err)
	}
}
