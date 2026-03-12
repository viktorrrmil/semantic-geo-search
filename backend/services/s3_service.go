package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Entry struct {
	Key  string `json:"key"`
	Type string `json:"type"` // "file" or "folder"
	Size *int64 `json:"size,omitempty"`
}

// newAnonymousS3Client creates an S3 client that works with public buckets
// without requiring AWS credentials.
func newAnonymousS3Client(ctx context.Context, region string) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}

// ListS3Entries lists the immediate children (files and folders) of a given
// S3 bucket + prefix.  It behaves like a single directory listing:
//   - CommonPrefixes → type "folder"
//   - Objects (excluding the prefix itself) → type "file"
func ListS3Entries(ctx context.Context, bucket, prefix, region string) ([]S3Entry, error) {
	client, err := newAnonymousS3Client(ctx, region)
	if err != nil {
		return nil, err
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var entries []S3Entry
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects in s3://%s/%s: %w", bucket, prefix, err)
		}

		for _, cp := range page.CommonPrefixes {
			entries = append(entries, S3Entry{
				Key:  aws.ToString(cp.Prefix),
				Type: "folder",
			})
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if key == prefix {
				continue
			}
			entries = append(entries, S3Entry{
				Key:  key,
				Type: "file",
				Size: obj.Size,
			})
		}
	}

	return entries, nil
}
