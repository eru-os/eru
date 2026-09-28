package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/segmentio/ksuid"
)

const (
	AuthTypeSecret = "SECRET"
	AuthTypeIAM    = "IAM"
)

const iv = "0123456789ABCDEF"

type AwsStorage struct {
	Storage
	Region         string `json:"region" eru:"required"`
	BucketName     string `json:"bucket_name" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	Key            string `json:"key" eru:"required"`
	Secret         string `json:"secret" eru:"required"`
	session        *s3.Client
}

func (awsStorage *AwsStorage) GetAttribute(attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "storage_name":
		return awsStorage.StorageName, nil
	case "storage_type":
		return awsStorage.StorageType, nil
	case "key_pair":
		return awsStorage.KeyPair, nil
	case "key_id":
		return awsStorage.KmsId, nil
	case "bucket_name":
		return awsStorage.BucketName, nil
	case "region":
		return awsStorage.Region, nil
	default:
		return nil, errors.New("Attribute not found")
	}
}

func (awsStorage *AwsStorage) DownloadFile(ctx context.Context, projectId string, folderPath string, fileName string, keyName eruaes.AesKey) (file []byte, err error) {
	_ = projectId
	logs.WithContext(ctx).Debug("DownloadFile - Start")
	if awsStorage.session == nil {
		err = awsStorage.Init(ctx)
		if err != nil {
			return
		}
	}

	result, err := awsStorage.session.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(awsStorage.BucketName),
		Key:    aws.String(fmt.Sprint(folderPath, "/", fileName)),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	defer result.Body.Close()

	byteContainer, err := io.ReadAll(result.Body)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	if awsStorage.EncryptFiles {
		var byteContainerKey []byte
		byteContainerSlice := bytes.Split(byteContainer, []byte("___eru___"))
		if len(byteContainerSlice) > 1 {
			byteContainerKey = byteContainerSlice[1]
		}
		byteContainer = byteContainerSlice[0]
		byteContainer, err = awsStorage.decrypt(ctx, byteContainer, byteContainerKey, keyName)
		if err != nil {
			return
		}
	}
	return byteContainer, nil
}

func (awsStorage *AwsStorage) UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	_ = projectId
	logs.WithContext(ctx).Debug("UploadFile - Start")
	if awsStorage.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = awsStorage.Init(ctx)
		if err != nil {
			return
		}
	}
	var byteContainer []byte
	enc := ""
	byteContainer, err = io.ReadAll(file)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if awsStorage.EncryptFiles {
		enc = ".enc"
		var byteContainerKey []byte
		byteContainer, byteContainerKey, err = awsStorage.encrypt(ctx, byteContainer, keyName)
		if err != nil {
			return
		}
		byteContainer = append(byteContainer, []byte("___eru___")...)
		byteContainer = append(byteContainer, byteContainerKey...)
	}

	docId = ksuid.New().String()
	if docType != "" {
		docType = fmt.Sprint(docType, "_")
	}
	finalFileName := fmt.Sprint(docType, docId, "_", header.Filename, enc)
	_, err = awsStorage.session.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(awsStorage.BucketName),
		Key:    aws.String(fmt.Sprint(folderPath, "/", finalFileName)),
		Body:   bytes.NewReader(byteContainer),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (awsStorage *AwsStorage) UploadFileB64(ctx context.Context, projectId string, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	_ = projectId
	logs.WithContext(ctx).Debug("UploadFileB64 - Start")
	if awsStorage.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = awsStorage.Init(ctx)
		if err != nil {
			return
		}
	}
	enc := ""

	if awsStorage.EncryptFiles {
		enc = ".enc"
		var fileKey []byte
		file, fileKey, err = awsStorage.encrypt(ctx, file, keyName)
		if err != nil {
			return
		}
		file = append(file, []byte("___eru___")...)
		file = append(file, fileKey...)
	}
	docId = ksuid.New().String()
	if docType != "" {
		docType = fmt.Sprint(docType, "_")
	}
	finalFileName := fmt.Sprint(docType, docId, "_", fileName, enc)

	_, err = awsStorage.session.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(awsStorage.BucketName),
		Key:    aws.String(fmt.Sprint(folderPath, "/", finalFileName)),
		Body:   bytes.NewReader(file),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	logs.WithContext(ctx).Info("file upload completed")
	return
}

func (awsStorage *AwsStorage) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(awsStorage.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if awsStorage.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			awsStorage.Key,
			awsStorage.Secret,
			"", // a token will be created when the session is used.
		)
	} else if awsStorage.Authentication == AuthTypeIAM {
		logs.WithContext(ctx).Info("connecting AWS S3 with IAM role")
		// do nothing - no new attributes to set in config
	}
	awsStorage.session = s3.NewFromConfig(awsConf)

	return err
}

func (awsStorage *AwsStorage) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &awsStorage)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (awsStorage *AwsStorage) CreateStorage(ctx context.Context, projectId string, cloneStorage StorageI, persist bool) (err error) {
	_ = projectId

	if awsStorage.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = awsStorage.Init(ctx)
		if err != nil {
			return
		}
	}
	if persist {
		be := false
		be, err = cloneStorage.BucketExists(ctx)
		if err != nil {
			return
		}

		if !be {
			bn, _ := cloneStorage.GetAttribute("bucket_name")
			rg, _ := cloneStorage.GetAttribute("region")

			logs.WithContext(ctx).Info(bn.(string))
			logs.WithContext(ctx).Info(rg.(string))

			_, err = awsStorage.session.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(bn.(string)),
				CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
					LocationConstraint: s3types.BucketLocationConstraint(rg.(string)),
				},
			})
			if err != nil {
				logs.WithContext(ctx).Info(err.Error())
				err = errors.New("error while creating new AWS bucket")
				return
			}
		} else {
			logs.WithContext(ctx).Info("skipping bucket creation in AWS as it already exists")
		}
	}
	return
}

func (awsStorage *AwsStorage) BucketExists(ctx context.Context) (exists bool, err error) {

	if awsStorage.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = awsStorage.Init(ctx)
		if err != nil {
			return
		}
	}
	logs.WithContext(ctx).Info(awsStorage.BucketName)
	_, err = awsStorage.session.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(awsStorage.BucketName),
	})

	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "NotFound", "NoSuchBucket":
				return false, nil // Bucket does not exist
			}
		}
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New(fmt.Sprint("error occurred while checking bucket : ", awsStorage.BucketName))
		return false, err
	}
	return true, err // Bucket exists
}
func (awsStorage *AwsStorage) DeleteStorage(ctx context.Context, projectId string, forceDelete bool, cloneStorage StorageI) (err error) {
	_ = projectId

	if awsStorage.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = awsStorage.Init(ctx)
		if err != nil {
			return
		}
	}
	if forceDelete {
		err = cloneStorage.EmptyBucket(ctx, projectId)
		if err != nil {
			return
		}
	}

	bn, _ := cloneStorage.GetAttribute("bucket_name")

	_, err = awsStorage.session.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bn.(string)),
	})

	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New("error while deleting AWS bucket")
		return
	}

	return
}

func (awsStorage *AwsStorage) EmptyBucket(ctx context.Context, projectId string) (err error) {
	_ = projectId
	paginator := s3.NewListObjectsV2Paginator(awsStorage.session, &s3.ListObjectsV2Input{
		Bucket: aws.String(awsStorage.BucketName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, obj := range page.Contents {
			_, delErr := awsStorage.session.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(awsStorage.BucketName),
				Key:    obj.Key,
			})
			if delErr != nil {
				return delErr
			}
		}
	}

	return nil
}

func (awsStorage AwsStorage) encrypt(ctx context.Context, byteContainer []byte, keyName eruaes.AesKey) (eByteContainer []byte, eByteContainerKey []byte, err error) {
	if awsStorage.KeyPair != "" {
		byteContainer = eruaes.Pad(byteContainer, 16)
		eByteContainer, err = eruaes.EncryptCBC(ctx, byteContainer, keyName.Key, keyName.Vector)
		if err != nil {
			return
		}
	} else if awsStorage.KmsKey != nil {
		aKey := eruaes.AesKey{}
		aKey, err = eruaes.GenerateKey(ctx, 16)
		if err != nil {
			return
		}

		byteContainer = eruaes.Pad(byteContainer, 16)
		eByteContainer, err = eruaes.EncryptCBC(ctx, byteContainer, aKey.Key, []byte(iv))
		if err != nil {
			return
		}

		eByteContainerKey, err = awsStorage.KmsKey.Encrypt(ctx, aKey.Key)
		if err != nil {
			return
		}
	} else {
		err = errors.New("encryption key not found")
		return
	}
	return
}

func (awsStorage AwsStorage) decrypt(ctx context.Context, eByteContainer []byte, byteContainerKey []byte, keyName eruaes.AesKey) (byteContainer []byte, err error) {
	if awsStorage.KeyPair != "" {
		byteContainer, err = eruaes.DecryptCBC(ctx, eByteContainer, keyName.Key, keyName.Vector)
		if err != nil {
			return
		}
		byteContainer, err = eruaes.Unpad(byteContainer)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	} else if awsStorage.KmsKey != nil {
		byteContainerKey, err = awsStorage.KmsKey.Decrypt(ctx, byteContainerKey)
		if err != nil {
			return
		}

		byteContainer, err = eruaes.DecryptCBC(ctx, eByteContainer, byteContainerKey, []byte(iv))
		if err != nil {
			return
		}
		byteContainer, err = eruaes.Unpad(byteContainer)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}

	} else {
		err = errors.New("encryption key not found")
		return
	}
	return
}
