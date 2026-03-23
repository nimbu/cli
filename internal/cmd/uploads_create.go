package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsCreateCmd uploads a file.
type UploadsCreateCmd struct {
	File        string   `arg:"" help:"Path to file to upload"`
	Name        string   `help:"Override filename" short:"n"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=custom.jpg)"`
}

// Run executes the create command.
func (c *UploadsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "upload file"); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	meta := map[string]any{}
	if c.Name != "" {
		meta["name"] = c.Name
	}
	if len(c.Assignments) > 0 {
		inlineBody, err := parseInlineAssignments(c.Assignments)
		if err != nil {
			return err
		}
		meta, err = mergeJSONBodies(inlineBody, meta)
		if err != nil {
			return fmt.Errorf("merge inline assignments: %w", err)
		}
		for key := range meta {
			if key != "name" {
				return fmt.Errorf("uploads create only supports name=<value> inline assignment")
			}
		}
	}

	// Determine filename
	filename := ""
	if rawName, exists := meta["name"]; exists {
		name, ok := rawName.(string)
		if !ok {
			return fmt.Errorf("uploads create requires name to be a string")
		}
		filename = name
	}
	if filename == "" {
		filename = filepath.Base(c.File)
	}

	task := output.ProgressFromContext(ctx).Transfer("upload "+filename, 0)
	body, err := newMultipartFileBody(c.File, filename, task)
	if err != nil {
		task.Fail(err)
		return err
	}
	var upload api.Upload
	if err := client.Post(ctx, "/uploads", body, &upload); err != nil {
		task.Fail(err)
		return fmt.Errorf("upload file: %w", err)
	}
	task.Done("done")

	return output.Print(ctx, upload, []any{upload.ID, upload.Name, upload.URL}, func() error {
		if _, err := output.Fprintf(ctx, "Uploaded: %s\n", upload.Name); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "ID:       %s\n", upload.ID); err != nil {
			return err
		}
		if upload.URL != "" {
			if _, err := output.Fprintf(ctx, "URL:      %s\n", upload.URL); err != nil {
				return err
			}
		}
		return nil
	})
}

func newMultipartFileBody(path string, filename string, task *output.Task) (api.RequestBody, error) {
	info, err := os.Stat(path)
	if err != nil {
		return api.RequestBody{}, fmt.Errorf("stat file: %w", err)
	}

	headerBuf := &bytes.Buffer{}
	writer := multipart.NewWriter(headerBuf)
	if _, err := writer.CreateFormFile("file", filename); err != nil {
		return api.RequestBody{}, fmt.Errorf("create form file: %w", err)
	}
	headerLen := headerBuf.Len()
	if err := writer.Close(); err != nil {
		return api.RequestBody{}, fmt.Errorf("close multipart writer: %w", err)
	}
	payload := headerBuf.Bytes()
	header := append([]byte(nil), payload[:headerLen]...)
	footer := append([]byte(nil), payload[headerLen:]...)
	contentType := writer.FormDataContentType()
	contentLength := int64(len(header)) + info.Size() + int64(len(footer))
	task.SetTotal(contentLength)

	buildReader := func(reset bool) (io.ReadCloser, error) {
		if reset {
			task.ResetProgress()
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		reader := io.MultiReader(bytes.NewReader(header), file, bytes.NewReader(footer))
		return &multipartBodyReadCloser{Reader: task.WrapReader(reader), file: file}, nil
	}

	reader, err := buildReader(false)
	if err != nil {
		return api.RequestBody{}, err
	}

	return api.RequestBody{
		Reader: reader,
		GetBody: func() (io.ReadCloser, error) {
			return buildReader(true)
		},
		ContentType:   contentType,
		ContentLength: contentLength,
	}, nil
}

type multipartBodyReadCloser struct {
	io.Reader
	file *os.File
}

func (r *multipartBodyReadCloser) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}
