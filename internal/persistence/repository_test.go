package persistence

import (
	"testing"
	"time"

	"wildframe/internal/domain"
)

func TestRepositoryPersistsAndReplays(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = repo.Update("created", "c1", time.Now(), func(state *State) error {
		state.Collections["c1"] = domain.ImageCollection{CollectionID: "c1", Version: 1}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = reopened.Read(func(state State) error {
		if state.Collections["c1"].Version != 1 {
			t.Fatal("projection missing")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
