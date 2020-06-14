package main

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

var (
	s3Contents = []*s3.Object{
		{
			Key: aws.String("mock-config/"),
		},
		{
			Key: aws.String("mock-config/openapi-mock.yaml"),
		},
		{
			Key: aws.String("mock-config/spec/"),
		},
		{
			Key: aws.String("mock-config/spec/spec-mock.yaml"),
		},
	}
)

type s3MockSvc struct {
	s3iface.S3API
}

func (s s3MockSvc) GetObject(input *s3.GetObjectInput) (output *s3.GetObjectOutput, err error) {
	body := ioutil.NopCloser(strings.NewReader("Hello World"))

	return &s3.GetObjectOutput{
		Body: body,
	}, nil
}

func (s s3MockSvc) ListObjectsV2(input *s3.ListObjectsV2Input) (output *s3.ListObjectsV2Output, err error) {
	if *input.Bucket != "my-bucket" {
		return nil, errors.New("bucket not found")
	}

	return &s3.ListObjectsV2Output{
		Contents: s3Contents,
	}, nil
}

func Test_loadConfigFromS3(t *testing.T) {
	defer os.RemoveAll("mock-config")

	type args struct {
		s3Svc  s3iface.S3API
		bucket string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Missing prefix",
			args: struct {
				s3Svc  s3iface.S3API
				bucket string
			}{
				s3Svc:  s3MockSvc{},
				bucket: "my-bucket-does-not-exist/config",
			},
			wantErr: true,
		},
		{
			name: "Download config files",
			args: struct {
				s3Svc  s3iface.S3API
				bucket string
			}{
				s3Svc:  s3MockSvc{},
				bucket: "s3://my-bucket",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := loadConfigFromS3(tt.args.s3Svc, tt.args.bucket); (err != nil) != tt.wantErr {
				t.Errorf("loadConfigFromS3() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
