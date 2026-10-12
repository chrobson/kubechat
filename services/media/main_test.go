package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	media "kubechat/proto/media"
)

type putCall struct {
	bucket, object string
	body           []byte
	size           int64
	opts           minio.PutObjectOptions
}

type fakeStore struct {
	puts    []putCall
	putErr  error
	stat    minio.ObjectInfo
	statErr error
}

func (f *fakeStore) PutObject(_ context.Context, bucket, object string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	body, _ := io.ReadAll(r)
	f.puts = append(f.puts, putCall{bucket, object, body, size, opts})
	return minio.UploadInfo{}, f.putErr
}

func (f *fakeStore) StatObject(_ context.Context, _, _ string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return f.stat, f.statErr
}

func newTestServer(store *fakeStore) *server {
	return &server{minioClient: store, bucketName: "media", publicURL: "http://cdn.test"}
}

func multipartBody(t *testing.T, field, filename, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	w.Close()
	return &buf, w.FormDataContentType()
}

func TestHandleUpload(t *testing.T) {
	store := &fakeStore{}
	s := newTestServer(store)

	body, ct := multipartBody(t, "file", "cat.png", "image/png", []byte("PNGDATA"))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	s.handleUpload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if len(store.puts) != 1 {
		t.Fatalf("PutObject calls = %d, want 1", len(store.puts))
	}
	put := store.puts[0]
	if put.bucket != "media" || !strings.HasSuffix(put.object, ".png") {
		t.Errorf("stored as %s/%s", put.bucket, put.object)
	}
	if string(put.body) != "PNGDATA" || put.size != int64(len("PNGDATA")) {
		t.Errorf("body = %q size = %d", put.body, put.size)
	}
	if put.opts.ContentType != "image/png" || put.opts.UserMetadata["Originalname"] != "cat.png" {
		t.Errorf("opts = %+v", put.opts)
	}

	var resp struct {
		MediaID string `json:"media_id"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, rec.Body)
	}
	if resp.MediaID != put.object || resp.URL != "http://cdn.test/media/"+put.object {
		t.Errorf("response = %+v", resp)
	}
}

func TestHandleUploadErrors(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		field      string
		putErr     error
		wantStatus int
	}{
		{"wrong method", http.MethodGet, "file", nil, http.StatusMethodNotAllowed},
		{"missing file field", http.MethodPost, "other", nil, http.StatusBadRequest},
		{"storage failure", http.MethodPost, "file", errors.New("minio down"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(&fakeStore{putErr: tc.putErr})
			body, ct := multipartBody(t, tc.field, "a.txt", "text/plain", []byte("x"))
			req := httptest.NewRequest(tc.method, "/upload", body)
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()
			s.handleUpload(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestGetMetadata(t *testing.T) {
	uploaded := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{stat: minio.ObjectInfo{
		ContentType:  "image/png",
		Size:         42,
		LastModified: uploaded,
		UserMetadata: minio.StringMap{"Originalname": "cat.png"},
	}}
	s := newTestServer(store)

	meta, err := s.GetMetadata(context.Background(), &media.GetMetadataRequest{MediaId: "abc.png"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.MediaId != "abc.png" || meta.OriginalName != "cat.png" || meta.MimeType != "image/png" || meta.Size != 42 {
		t.Errorf("metadata = %+v", meta)
	}
	if meta.Url != "http://cdn.test/media/abc.png" {
		t.Errorf("url = %q", meta.Url)
	}
	if !meta.UploadedAt.AsTime().Equal(uploaded) {
		t.Errorf("uploaded_at = %v", meta.UploadedAt.AsTime())
	}

	store.statErr = errors.New("not found")
	if _, err := s.GetMetadata(context.Background(), &media.GetMetadataRequest{MediaId: "missing"}); err == nil {
		t.Error("expected error for missing object")
	}
}
