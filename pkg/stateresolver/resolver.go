// State resolver is used to compute the state of collection at an exact revision. The
// revision's parent DAG is traversed back to the root to compute all the deltas.
package stateresolver

import (
	"errors"
	"github.com/function61/varasto/pkg/varastotypes"
	"sort"
)

type fileMap map[string]varastotypes.File

type StateAt struct {
	ChangesetId string
	files       fileMap
}

func (s *StateAt) Files() fileMap {
	files := fileMap{}

	// makes a clone
	for key, value := range s.files {
		files[key] = value
	}

	return files
}

// List of files present at this revision
func (s *StateAt) FileList() []varastotypes.File {
	files := []varastotypes.File{}

	for _, file := range s.files {
		files = append(files, file)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files
}

func ComputeStateAt(c varastotypes.Collection, changesetId string) (*StateAt, error) {
	state := &StateAt{
		ChangesetId: changesetId,
		files:       fileMap{},
	}

	// initial state is always empty
	if changesetId == varastotypes.NoParentId {
		return state, nil
	}

	ch := findChangesetById(c, changesetId)
	if ch == nil {
		return nil, errors.New("changeset not found")
	}

	parents := []*varastotypes.CollectionChangeset{ch}

	curr := ch

	for curr.Parent != varastotypes.NoParentId {
		parent := findChangesetById(c, curr.Parent)
		if parent == nil {
			return nil, errors.New("parent not found")
		}

		parents = append(parents, parent)

		curr = parent
	}

	for i := len(parents) - 1; i >= 0; i-- {
		parent := parents[i]

		for _, add := range parent.FilesCreated {
			state.files[add.Path] = add
		}
		for _, remove := range parent.FilesDeleted {
			delete(state.files, remove)
		}
		for _, update := range parent.FilesUpdated {
			state.files[update.Path] = update
		}
	}

	return state, nil
}

func findChangesetById(c varastotypes.Collection, id string) *varastotypes.CollectionChangeset {
	for _, changeset := range c.Changesets {
		if changeset.ID == id {
			return &changeset
		}
	}

	return nil
}
