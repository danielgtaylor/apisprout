package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

func loadConfigFromS3WithSession(bucket string) (err error) {
	sess, err := session.NewSession()
	if err != nil {
		return
	}

	s3Svc := s3.New(sess)

	return loadConfigFromS3(s3Svc, bucket)
}

func loadConfigFromS3(s3Svc s3iface.S3API, bucket string) (err error) {
	if !strings.HasPrefix(bucket, "s3://") {
		return errors.New("invalid bucket name, missing prefix")
	}

	s3Uri, err := url.Parse(bucket)
	if err != nil {
		return
	}

	s3Objects, err := s3Svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(s3Uri.Host),
		Prefix: aws.String(s3Uri.Path),
	})

	if err != nil {
		return
	}

	for _, content := range s3Objects.Contents {
		fmt.Printf("Getting file %s from bucket...", *content.Key)
		if strings.HasSuffix(*content.Key, "/") {
			continue
		}

		err = os.MkdirAll(filepath.Dir(*content.Key), 0755)

		if err != nil {
			return
		}

		s3ObjectData, err := s3Svc.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(s3Uri.Host),
			Key:    content.Key,
		})

		if err != nil {
			return err
		}

		s3FileBody, err := ioutil.ReadAll(s3ObjectData.Body)
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(*content.Key, s3FileBody, 0644)
		if err != nil {
			return err
		}
		fmt.Println(" Done")
	}

	return
}
