package main

import (
	"errors"
	"fmt"

	"github.com/ostafen/clover"
)

type store struct {
	db *clover.DB
}

type collectionName string

const (
	collectionGists       collectionName = "gists"
	collectionGistContent collectionName = "gist_content_list"
)

var (
	collections = []collectionName{
		collectionGists,
		collectionGistContent,
	}
)

func (s *store) init(path string) error {
	db, err := clover.Open(path)
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to check collection: %v", err))
	}
	s.db = db
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
