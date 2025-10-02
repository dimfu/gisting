package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v74/github"
	"github.com/google/uuid"
	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.design/x/clipboard"
)

var (
	cfg *config
	log = logrus.New()

	storage       = new(store)
	withVimMotion = false
)

func init() {
	if err := setup(); err != nil {
		panic(err)
	}
	err := clipboard.Init()
	if err != nil {
		panic(err)
	}
}

func main() {
	defer storage.db.Close()
	f, err := initLogger()
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	cmd := &cli.Command{
		Name:    "gisting",
		Usage:   "interactive gist management in tui",
		Version: "1.0.3",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "theme",
				Aliases: []string{"t"},
				Value:   cfg.Theme,
			},
			&cli.BoolFlag{
				Name:    "vimmotion",
				Aliases: []string{"vm"},
				Usage:   "using vim motion",
				Value:   false,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			withVimMotion = c.Bool("vimmotion")
			theme := c.String("theme")
			cfg.set("Theme", theme)
			cfg.Theme = theme
			p := tea.NewProgram(initialModel(), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return err
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "List authed user gist",
				Action:  fileList,
			},
			{
				Name:    "create",
				Aliases: []string{"c"},
				Usage:   "Create new gist",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "description",
						Aliases: []string{"d"},
						Value:   "",
						Usage:   "Description for this gist",
					},
					&cli.BoolFlag{
						Name:    "clipboard",
						Aliases: []string{"P"},
						Value:   false,
						Usage:   "Create file with content from clipboard",
					},
					&cli.StringFlag{
						Name:    "filename",
						Aliases: []string{"f"},
						Value:   "",
						Usage:   "Set filename for the gist file",
					},
				},
				Action: create,
			},
			{
				Name:  "delete",
				Usage: "Delete a gist or file (gisting [GIST_ID] [FILE_NAME])",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "type",
						Aliases: []string{"t"},
						Value:   "file",
					},
				},
				Action: delete,
			},
			{
				Name:    "drop",
				Aliases: []string{"d"},
				Usage:   "Drop all collection records",
				Action: func(ctx context.Context, c *cli.Command) error {
					if err := storage.drop(); err != nil {
						return err
					}
					fmt.Println("Collections dropped successfully")
					return nil
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Println(err)
	}
}

func fileList(ctx context.Context, c *cli.Command) error {
	if !cfg.hasAccessToken() {
		return err_unauthorized
	}
	client := github.NewClient(nil).WithAuthToken(cfg.AccessToken)
	out := []string{}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	gists, _, err := client.Gists.List(ctx, "", &github.GistListOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})

	if err != nil {
		var errRes *github.ErrorResponse
		if errors.As(err, &errRes) {
			return errors.New(errRes.Message)
		}
		return err
	}

	for _, gist := range gists {
		for _, file := range gist.Files {
			out = append(out, fmt.Sprintf("%s\t%s\t%s\n", gist.GetID(), gist.GetDescription(), file.GetFilename()))
		}
	}

	draftedDocs, err := storage.db.FindAll(
		query.NewQuery(string(collectionDraftedGists)),
	)
	if err != nil {
		return err
	}

	for _, doc := range draftedDocs {
		gistId := doc.Get("id").(string)
		name := doc.Get("description").(string)
		filename := doc.Get("title").(string)

		out = append(out, fmt.Sprintf("%s\t%s\t%s\n", gistId, name, filename))
	}
	fmt.Fprintln(w, "ID\tNAME\tFILENAME")
	for _, row := range out {
		fmt.Fprint(w, row)
	}
	w.Flush()
	return nil
}

func create(ctx context.Context, c *cli.Command) error {
	if !cfg.hasAccessToken() {
		return err_unauthorized
	}
	client := github.NewClient(nil).WithAuthToken(cfg.AccessToken)
	_, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return err
	}

	public := true
	gist := github.Gist{
		Public: &public,
		Files:  map[github.GistFilename]github.GistFile{},
	}

	description := c.String("description")
	if description != "" {
		gist.Description = &description
	}

	var out string
	fromClipboard := c.Bool("clipboard")

	files := []file{}

	if fromClipboard {
		content := string(clipboard.Read(clipboard.FmtText))
		filename := c.String("filename")
		gist.Files[github.GistFilename(filename)] = github.GistFile{
			Filename: &filename,
			Content:  &content,
		}
	} else {
		for i := 0; i < c.Args().Len(); i++ {
			path := c.Args().Get(i)
			f, err := os.Stat(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					out += fmt.Sprintf("cannot add %q: no such file", path)
					continue
				}
				log.Println(err)
				return err
			}

			// ignore directory for now
			if f.IsDir() {
				out += fmt.Sprintf("could not add directory %q as gist file", f.Name())
				continue
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			filename := f.Name()

			// only allows changing filename if only 1 file is included as the args
			if c.Args().Len() == 1 && c.String("filename") != "" {
				filename = c.String("filename")
			}

			content := string(data)

			files = append(files, file{})

			gist.Files[github.GistFilename(f.Name())] = github.GistFile{
				Filename: &filename,
				Content:  &content,
			}
		}
	}

	createdGist, _, err := client.Gists.Create(context.Background(), &gist)
	if err != nil {
		var errRes *github.ErrorResponse
		if errors.As(err, &errRes) {
			return errors.New(errRes.Message)
		}
		return err
	}

	for _, uploadedFile := range createdGist.Files {
		for _, file := range gist.Files {
			if uploadedFile.GetFilename() == file.GetFilename() {
				doc := document.NewDocument()
				doc.SetAll(map[string]any{
					"id":        uuid.New().String(),
					"gistId":    createdGist.GetID(),
					"title":     file.GetFilename(),
					"rawUrl":    uploadedFile.GetRawURL(),
					"updatedAt": createdGist.GetUpdatedAt().Time.In(time.Local).String(),
					"content":   file.GetContent(),
					"draft":     false,
				})
				err := storage.db.Insert(string(collectionGistContent), doc)
				if err != nil {
					return fmt.Errorf("failed to insert file %q: %w", file.GetFilename(), err)
				}
				out += fmt.Sprintf("%q added to the gist %q\n", file.GetFilename(), createdGist.GetDescription())
			}
		}
	}

	fmt.Println(strings.TrimRight(out, "\n"))

	return nil
}

func delete(ctx context.Context, c *cli.Command) error {
	if !cfg.hasAccessToken() {
		return err_unauthorized
	}

	gistId := c.Args().Get(0)
	filename := c.Args().Get(1)

	client := github.NewClient(nil).WithAuthToken(cfg.AccessToken)
	_, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return err
	}

	t := c.String("type")

	switch t {
	case "gist":
		_, err := client.Gists.Delete(context.Background(), gistId)
		if err != nil {
			var errRes *github.ErrorResponse
			if errors.As(err, &errRes) {
				return errors.New(errRes.Message)
			}
			return err
		}
		fmt.Printf("%q gist successfully deleted\n", gistId)
		break
	case "file":
		gist := github.Gist{
			Files: map[github.GistFilename]github.GistFile{
				github.GistFilename(filename): {},
			},
		}
		_, _, err := client.Gists.Edit(context.Background(), gistId, &gist)
		if err != nil {
			var errRes *github.ErrorResponse
			if errors.As(err, &errRes) {
				return errors.New(errRes.Message)
			}
			return err
		}
		fmt.Printf("%q gist file successfully deleted\n", filename)
		break
	default:
		return fmt.Errorf("Delete type %q is not recognized\n", t)
	}

	return nil
}
