package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ostafen/clover/v2/query"
)

type file struct {
	id        string `clover:"id"`
	gistId    string `clover:"gistId"`
	title     string `clover:"title"`
	desc      string `clover:"desc"`
	rawUrl    string `clover:"rawUrl"`
	updatedAt string `clover:"updatedAt"`
	content   string `clover:"content"`
	draft     bool   `clover:"draft"`
	stale     bool
}

type files struct {
	files []file
}

func (f file) Title() string       { return f.title }
func (f file) Description() string { return f.desc }
func (f file) FilterValue() string { return f.title }

func (f file) getContent() (string, error) {
	if f.draft {
		return f.content, nil
	}

	existing, err := storage.db.FindFirst(
		query.NewQuery(string(collectionGistContent)).Where(query.Field("rawUrl").Eq(f.rawUrl).And(query.Field("id").Eq(f.id))),
	)

	if err != nil {
		return "", err
	}

	var shouldFetch bool
	existingUA, _ := existing.Get("updatedAt").(string)
	shouldFetch = f.updatedAt > existingUA

	if _, ok := existing.Get("content").(string); !ok {
		shouldFetch = true
	}

	if shouldFetch {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(f.rawUrl)
		if err != nil {
			logs = append(logs, fmt.Sprintf("Could not fetch file with raw url: %s", f.rawUrl))
			return "", err
		}
		defer resp.Body.Close()

		contentBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logs = append(logs, err.Error())
			return "", err
		}

		content := string(contentBytes)

		existing.Set("content", content)

		if err := storage.db.Save(string(collectionGistContent), existing); err != nil {
			logs = append(logs, err.Error())
			return "", err
		}

		return content, nil
	} else {
		if cachedContent, ok := existing.Get("content").(string); ok {
			return cachedContent, nil
		}
		return "", fmt.Errorf("no cached content available")
	}
}
