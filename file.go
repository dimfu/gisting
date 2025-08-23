package main

import (
	"io"
	"net/http"
	"time"

	"github.com/ostafen/clover"
)

type file struct {
	title     string `clover:"title"`
	desc      string `clover:"desc"`
	rawUrl    string `clover:"rawUrl"`
	updatedAt string `clover:"updatedAt"`
	stale     bool
}

type files struct {
	files []file
}

func (f file) Title() string       { return f.title }
func (f file) Description() string { return f.desc }
func (f file) FilterValue() string { return f.title }

func (f file) content() (string, error) {
	var content string

	existing, err := storage.db.Query(string(collectionGistContent)).
		Where(clover.Field("rawUrl").Eq(f.rawUrl)).
		FindFirst()
	if err != nil {
		logs = append(logs, err.Error())
		return "", err
	}

	if f.stale || existing == nil {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(f.rawUrl)
		if err != nil {
			logs = append(logs, err.Error())
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
			existing = clover.NewDocument()
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
