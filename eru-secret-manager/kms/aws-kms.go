package kms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type AwsKmsStore struct {
	KmsStore
	Region         string `json:"region" eru:"required"`
	KmsName        string `json:"kms_name"`
	KmsDesc        string `json:"kms_desc" eru:"required"`
	KmsAlias       string `json:"kms_alias" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	Key            string `json:"key" eru:"required"`
	Secret         string `json:"secret" eru:"required"`
	client         *kms.Client
}

func (awsKmsStore *AwsKmsStore) GetAttribute(ctx context.Context, attrName string) (attrValue interface{}, err error) {
	switch attrName {
	case "region":
		return awsKmsStore.Region, nil
	case "kms_name":
		return awsKmsStore.KmsName, nil
	case "kms_desc":
		return awsKmsStore.KmsDesc, nil
	case "kms_alias":
		return awsKmsStore.KmsAlias, nil
	case "kms_store_type":
		return awsKmsStore.KmsStoreType, nil

	default:
		err = errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
}

func (awsKmsStore *AwsKmsStore) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(ctx,
		config.WithRegion(awsKmsStore.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if awsKmsStore.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			awsKmsStore.Key,
			awsKmsStore.Secret,
			"", // a token will be created when the session it's used. //TODO to check this
		)
	}
	awsKmsStore.client = kms.NewFromConfig(awsConf)
	return err
}

func (awsKmsStore *AwsKmsStore) CreateKey(ctx context.Context) (err error) {
	if awsKmsStore.client == nil {
		err = awsKmsStore.Init(ctx)
		if err != nil {
			return
		}
	}
	aliasesPaginator := kms.NewListAliasesPaginator(awsKmsStore.client, &kms.ListAliasesInput{})

	aliasFound := false
	for aliasesPaginator.HasMorePages() {
		if aliasFound {
			break
		}
		aliasesPage, pageErr := aliasesPaginator.NextPage(ctx)
		if pageErr != nil {
			logs.WithContext(ctx).Error(pageErr.Error())
			var nfe *types.NotFoundException
			if errors.As(pageErr, &nfe) {
				break
			}
			return pageErr
		}
		for _, alias := range aliasesPage.Aliases {
			if fmt.Sprint("alias/", awsKmsStore.KmsAlias) == *alias.AliasName {
				logs.WithContext(ctx).Info(fmt.Sprint("key alias already exists : ", awsKmsStore.KmsAlias))
				aliasFound = true
				awsKmsStore.KmsName = *alias.TargetKeyId
				break
			}
		}
	}
	if !aliasFound {
		keyInput := &kms.CreateKeyInput{
			Description: aws.String(awsKmsStore.KmsDesc),
			KeySpec:     types.KeySpecSymmetricDefault,
			KeyUsage:    types.KeyUsageTypeEncryptDecrypt,
		}
		keyResult, err := awsKmsStore.client.CreateKey(ctx, keyInput)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		awsKmsStore.KmsName = *keyResult.KeyMetadata.KeyId

		// Create an alias for the newly created key
		aliasInput := &kms.CreateAliasInput{
			AliasName:   aws.String(fmt.Sprint("alias/", awsKmsStore.KmsAlias)),
			TargetKeyId: &awsKmsStore.KmsName,
		}
		_, err = awsKmsStore.client.CreateAlias(ctx, aliasInput)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	}
	return
}

func (awsKmsStore *AwsKmsStore) DeleteKey(ctx context.Context, keyId string, deleteDays int32) (err error) {
	if awsKmsStore.client == nil {
		err = awsKmsStore.Init(ctx)
		if err != nil {
			return
		}
	}
	deleteInput := &kms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(keyId),
		PendingWindowInDays: aws.Int32(deleteDays), // Minimum is 7 days
	}

	logs.WithContext(ctx).Info(fmt.Sprint(deleteInput))

	_, err = awsKmsStore.client.ScheduleKeyDeletion(ctx, deleteInput)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint("delete done"))
	return
}

func (awsKmsStore *AwsKmsStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &awsKmsStore)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (awsKmsStore *AwsKmsStore) Encrypt(ctx context.Context, plainText []byte) (encryptedText []byte, err error) {
	if awsKmsStore.client == nil {
		err = awsKmsStore.Init(ctx)
		if err != nil {
			return
		}
	}
	encryptInput := &kms.EncryptInput{
		KeyId:     aws.String(awsKmsStore.KmsName),
		Plaintext: plainText,
	}
	encryptOutput, err := awsKmsStore.client.Encrypt(ctx, encryptInput)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	encryptedText = encryptOutput.CiphertextBlob
	return
}

func (awsKmsStore *AwsKmsStore) Decrypt(ctx context.Context, encryptedText []byte) (plainText []byte, err error) {
	if awsKmsStore.client == nil {
		err = awsKmsStore.Init(ctx)
		if err != nil {
			return
		}
	}
	decryptInput := &kms.DecryptInput{
		KeyId:          aws.String(awsKmsStore.KmsName),
		CiphertextBlob: encryptedText,
	}
	decryptOutput, err := awsKmsStore.client.Decrypt(ctx, decryptInput)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	plainText = decryptOutput.Plaintext
	return
}
