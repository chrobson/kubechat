package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	media "kubechat/proto/media"
)

type server struct {
	media.UnimplementedMediaServiceServer
	minioClient *minio.Client
	bucketName  string
	publicURL   string
}

func (s *server) GetMetadata(ctx context.Context, req *media.GetMetadataRequest) (*media.MediaMetadata, error) {
	objInfo, err := s.minioClient.StatObject(ctx, s.bucketName, req.MediaId, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &media.MediaMetadata{
		MediaId:      req.MediaId,
		OriginalName: objInfo.UserMetadata["Originalname"],
		MimeType:     objInfo.ContentType,
		Size:         objInfo.Size,
		Url:          fmt.Sprintf("%s/%s/%s", s.publicURL, s.bucketName, req.MediaId),
		UploadedAt:   timestamppb.New(objInfo.LastModified),
	}, nil
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 10MB limit
	r.ParseMultipartForm(10 << 20)

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	mediaID := uuid.New().String() + filepath.Ext(handler.Filename)
	
	_, err = s.minioClient.PutObject(r.Context(), s.bucketName, mediaID, file, handler.Size, minio.PutObjectOptions{
		ContentType:  handler.Header.Get("Content-Type"),
		UserMetadata: map[string]string{"Originalname": handler.Filename},
	})
	if err != nil {
		log.Printf("Failed to upload to MinIO: %v", err)
		http.Error(w, "Upload failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"media_id": "%s", "url": "%s/%s/%s"}`, mediaID, s.publicURL, s.bucketName, mediaID)
}

func main() {
	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	if minioEndpoint == "" {
		minioEndpoint = "localhost:9000"
	}
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	useSSL := os.Getenv("MINIO_USE_SSL") == "true"
	bucketName := os.Getenv("MINIO_BUCKET")
	if bucketName == "" {
		bucketName = "kubechat-media"
	}
	publicURL := os.Getenv("PUBLIC_MEDIA_URL")
	if publicURL == "" {
		publicURL = "http://localhost:9000"
	}

	mc, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalf("Failed to initialize MinIO: %v", err)
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := mc.BucketExists(ctx, bucketName)
	if err != nil {
		log.Fatalf("Failed to check bucket: %v", err)
	}
	if !exists {
		err = mc.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Fatalf("Failed to create bucket: %v", err)
		}
		
		// Set bucket policy to public read
		policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, bucketName)
		err = mc.SetBucketPolicy(ctx, bucketName, policy)
		if err != nil {
			log.Printf("Failed to set bucket policy: %v", err)
		}
	}

	mediaServer := &server{
		minioClient: mc,
		bucketName:  bucketName,
		publicURL:   publicURL,
	}

	// HTTP Server for Uploads
	go func() {
		http.HandleFunc("/upload", mediaServer.handleUpload)
		log.Println("Media HTTP server listening on :8081")
		_ = http.ListenAndServe(":8081", nil)
	}()

	// gRPC Server for Metadata
	lis, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	media.RegisterMediaServiceServer(s, mediaServer)

	log.Println("Media gRPC service listening on :50055")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
