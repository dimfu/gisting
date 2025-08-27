package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
)

type file struct {
	title     string `clover:"title"`
	desc      string `clover:"desc"`
	rawUrl    string `clover:"rawUrl"`
	updatedAt string `clover:"updatedAt"`
	content   string `clover:"content"`
	stale     bool
	draft     bool
}

type files struct {
	files []file
}

func (f file) Title() string       { return f.title }
func (f file) Description() string { return f.desc }
func (f file) FilterValue() string { return f.title }

func (f file) getContent() (string, error) {
	var content string
	existing, err := storage.db.FindFirst(
		query.NewQuery(string(collectionGistContent)).Where(query.Field("rawUrl").Eq(f.rawUrl)),
	)
	if err != nil {
		logs = append(logs, fmt.Sprintf("Could not find file with url %s", f.rawUrl))
		return "", err
	}

	if f.draft {
		return f.content, nil
	}

	if f.stale || existing == nil {
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
		content = string(contentBytes)

		if existing == nil {
			existing = document.NewDocument()
			existing.Set("rawUrl", f.rawUrl)
		}

		existing.SetAll(map[string]any{
			"title":     f.title,
			"desc":      f.desc,
			"rawUrl":    f.rawUrl,
			"updatedAt": f.updatedAt,
			"content":   content,
		})

		if err := storage.db.Save(string(collectionGistContent), existing); err != nil {
			logs = append(logs, err.Error())
			return "", err
		}
	} else {
		if val, ok := existing.Get("content").(string); ok {
			content = val
		}
	}

	return content, nil
}
