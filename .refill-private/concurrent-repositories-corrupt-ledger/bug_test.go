package concurrentrepositoriescorruptledger

import (
	"sync"
	"testing"
	"time"

	"wildframe/internal/persistence"
)

func TestConcurrentRepositoryOwnersDoNotCorruptLedger(t *testing.T) {
	dir := t.TempDir()
	type owner struct {
		repo *persistence.Repository
		err  error
	}
	owners := make(chan owner, 2)
	start := make(chan struct{})
	var opened sync.WaitGroup
	opened.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			repo, err := persistence.Open(dir)
			owners <- owner{repo: repo, err: err}
			opened.Done()
			<-start
		}()
	}
	opened.Wait()
	close(start)
	first, second := <-owners, <-owners
	if first.err != nil || second.err != nil {
		return
	}
	updates := make(chan error, 2)
	for index, repo := range []*persistence.Repository{first.repo, second.repo} {
		index, repo := index, repo
		go func() {
			updates <- repo.Update("owner.updated", "collection", time.Unix(int64(index+1), 0), func(state *persistence.State) error {
				state.Idempotency[string(rune('a'+index))] = persistence.IdempotencyRecord{Fingerprint: "value"}
				return nil
			})
		}()
	}
	if err := <-updates; err != nil {
		t.Fatal(err)
	}
	if err := <-updates; err != nil {
		t.Fatal(err)
	}
	if _, err := persistence.Open(dir); err != nil {
		t.Fatalf("两个已打开的数据目录所有者使用各自缓存序号写出了不可恢复账本: %v", err)
	}
}
