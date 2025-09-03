package main

import (
	"errors"
	"fmt"

	c "github.com/ostafen/clover/v2"
)

type store struct {
	db *c.DB
}

type collectionName string

const (
	collectionGistContent  collectionName = "gist_content_list"
	collectionDraftedGists collectionName = "drafted_gists"
)

var (
	collections = []collectionName{
		collectionGistContent,
		collectionDraftedGists,
	}
)

func (s *store) init(path string, drop bool) error {
	db, err := c.Open(path)
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to check collection: %v", err))
	}
	s.db = db

	if drop {
		for _, collection := range collections {
			if err := db.DropCollection(string(collection)); err != nil {
				return fmt.Errorf("error while dropping collection %s\n", collection)
			}
		}
	}

	for _, collection := range collections {
		s.initCollection(collection)
	}

	return nil
}

func (s *store) initCollection(name collectionName) error {
	exists, err := s.db.HasCollection(string(name))
	if err != nil {
		return err
	}
	if !exists {
		s.db.CreateCollection(string(name))
		return nil
	}
	return nil
}
