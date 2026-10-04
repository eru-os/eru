package sm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type AwsSmStore struct {
	SmStore
	Region string `json:"region" eru:"required"`
	SmName string `json:"sm_name" eru:"required"`
	//KmsName        string `json:"kms_name" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	Key            string `json:"key" eru:"required"`
	Secret         string `json:"secret" eru:"required"`
	client         *secretsmanager.Client
}

func (awsSmStore *AwsSmStore) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(ctx,
		config.WithRegion(awsSmStore.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	//if awsSmStore.Authentication == AuthTypeIAM {
	//	appCreds := aws.NewCredentialsCache(ec2rolecreds.New())
	//	cred, err := appCreds.Retrieve(ctx)
	//	if err != nil {
	//		logs.WithContext(ctx).Error(err.Error())
	//	}
	//	logs.WithContext(ctx).Error(fmt.Sprint(cred))
	//} else
	if awsSmStore.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			awsSmStore.Key,
			awsSmStore.Secret,
			"", // a token will be created when the session it's used. //TODO to check this
		)
	}
	awsSmStore.client = secretsmanager.NewFromConfig(awsConf)
	return err
}
func (awsSmStore *AwsSmStore) InitCache(ctx context.Context, projectId string) (err error) {
	if awsSmStore.CacheStoreType == "" {
		awsSmStore.CacheStoreType = "INMEMORY"
	}
	awsSmStore.SetCacheStore(cache.GetCacheStore(awsSmStore.CacheStoreType, projectId))
	logs.WithContext(ctx).Info(fmt.Sprint(awsSmStore.CacheStore))
	return
}
func (awsSmStore *AwsSmStore) FetchSmValue(ctx context.Context) (resultJson map[string]string, err error) {
	logs.WithContext(ctx).Debug("FetchSmValue - Start")
	if awsSmStore.client == nil {
		err = awsSmStore.Init(ctx)
		if err != nil {
			return
		}
	}
	logs.WithContext(ctx).Info(awsSmStore.SmName)
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(awsSmStore.SmName),
		VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
	}

	result, err := awsSmStore.client.GetSecretValue(ctx, input)
	if err != nil {
		err = logs.Err(ctx, err, "failed to get secret value")
		return
	}
	err = json.Unmarshal([]byte(*result.SecretString), &resultJson)
	if err != nil {
		err = logs.Err(ctx, err, "failed to unmarshal secret value")
		resultJson["secret"] = *result.SecretString
		return
	}
	return
}

func (awsSmStore *AwsSmStore) SetSmValue(ctx context.Context, secretName string, secretJson map[string]string) (err error) {
	if awsSmStore.client == nil {
		err = awsSmStore.Init(ctx)
		if err != nil {
			return
		}
	}
	paginator := secretsmanager.NewListSecretsPaginator(awsSmStore.client, &secretsmanager.ListSecretsInput{})
	smFound := false
	secretId := ""
	for paginator.HasMorePages() {
		if smFound {
			break
		}
		page, err := paginator.NextPage(ctx)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		for _, secret := range page.SecretList {
			if aws.ToString(secret.Name) == secretName {
				smFound = true
				secretId = aws.ToString(secret.ARN)
				break
			}
		}
	}
	if smFound {
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     &secretId,
			VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
		}

		result, err := awsSmStore.client.GetSecretValue(ctx, input)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		resultJson := make(map[string]string)
		err = json.Unmarshal([]byte(*result.SecretString), &resultJson)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			resultJson["secret"] = *result.SecretString
			return err
		}

		for k, v := range secretJson {
			resultJson[k] = v
		}
		resultStr := ""
		resultBytes, err := json.Marshal(resultJson)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			resultStr = *result.SecretString
		} else {
			resultStr = string(resultBytes)
		}
		_, err = awsSmStore.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
			SecretId:     aws.String(secretId),
			SecretString: aws.String(resultStr),
		})
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	} else {
		resultStr := ""
		resultBytes, err := json.Marshal(secretJson)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		} else {
			resultStr = string(resultBytes)
		}
		_, err = awsSmStore.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         aws.String(secretName),
			SecretString: aws.String(resultStr),
		})
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	}
	return
}
func (awsSmStore *AwsSmStore) getSecretArn(ctx context.Context, secretName string) (secretArn string, smFound bool, err error) {
	if awsSmStore.client == nil {
		err = awsSmStore.Init(ctx)
		if err != nil {
			return
		}
	}
	paginator := secretsmanager.NewListSecretsPaginator(awsSmStore.client, &secretsmanager.ListSecretsInput{})
	smFound = false
	for paginator.HasMorePages() {
		if smFound {
			break
		}
		page, err := paginator.NextPage(ctx)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		for _, secret := range page.SecretList {
			if aws.ToString(secret.Name) == secretName {
				smFound = true
				secretArn = aws.ToString(secret.ARN)
				break
			}
		}
	}
	return
}
func (awsSmStore *AwsSmStore) DeleteSm(ctx context.Context, secretArn string) (err error) {
	if awsSmStore.client == nil {
		err = awsSmStore.Init(ctx)
		if err != nil {
			return
		}
	}
	deleteInput := &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretArn),
		ForceDeleteWithoutRecovery: aws.Bool(true), // Set to true to delete immediately without recovery
	}
	_, err = awsSmStore.client.DeleteSecret(ctx, deleteInput)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}
func (awsSmStore *AwsSmStore) UnsetSmValue(ctx context.Context, secretName string, secretKey string) (err error) {
	secretArn, smFound, secretErr := awsSmStore.getSecretArn(ctx, secretName)
	if secretErr != nil {
		return
	}
	if smFound {
		input := &secretsmanager.GetSecretValueInput{
			SecretId:     &secretArn,
			VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
		}

		result, err := awsSmStore.client.GetSecretValue(ctx, input)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		resultJson := make(map[string]string)
		err = json.Unmarshal([]byte(*result.SecretString), &resultJson)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			resultJson["secret"] = *result.SecretString
			return err
		}

		delete(resultJson, secretKey)
		if len(resultJson) == 0 {
			err = awsSmStore.DeleteSm(ctx, secretArn)
			return err
		}

		resultStr := ""
		resultBytes, err := json.Marshal(resultJson)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			resultStr = *result.SecretString
		} else {
			resultStr = string(resultBytes)
		}
		_, err = awsSmStore.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
			SecretId:     aws.String(secretArn),
			SecretString: aws.String(resultStr),
		})
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	} else {
		err = errors.New("secret manager not found")
	}
	return
}

func (awsSmStore *AwsSmStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &awsSmStore)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (awsSmStore *AwsSmStore) GetSmValue(ctx context.Context, projectId string, secretName string, secretKey string, forceFetch bool) (secretValue interface{}, err error) {
	logs.WithContext(ctx).Debug("GetSmValue - Start")

	if !forceFetch {
		logs.WithContext(ctx).Info(fmt.Sprint("fetch secret from cache for : ", secretKey))
		if awsSmStore.CacheStore == nil {
			logs.WithContext(ctx).Info("initializing sm cache")
			err = awsSmStore.InitCache(ctx, projectId)
			forceFetch = true //since cache is not initialized, fetch from cloud
		} else {
			secretValue, err = awsSmStore.CacheStore.Get(ctx, fmt.Sprint(secretName, "_", secretKey))
			if secretValue == "" {
				forceFetch = true //since fetching from cache failed, fetch from cloud
			}
		}
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	if err != nil || forceFetch {
		logs.WithContext(ctx).Info(fmt.Sprint("fetch secret from cloud for : ", secretKey))
		if awsSmStore.client == nil {
			err = awsSmStore.Init(ctx)
			if err != nil {
				return
			}
		}

		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(secretName),
			VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
		}

		result, err := awsSmStore.client.GetSecretValue(ctx, input)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		resultJson := make(map[string]string)
		err = json.Unmarshal([]byte(*result.SecretString), &resultJson)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			resultJson["secret"] = *result.SecretString
			return nil, err
		}
		svOk := false
		if secretValue, svOk = resultJson[secretKey]; svOk {
			// The cache is only set up on the cached path; a forced fetch may
			// arrive before it exists.
			if awsSmStore.CacheStore != nil {
				err = awsSmStore.CacheStore.Set(ctx, fmt.Sprint(secretName, "_", secretKey), secretValue)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					err = nil // exit silently
				}
			}
			return secretValue, nil
		} else {
			err = errors.New("secret key not found in secret manager")
			return nil, err
		}
	}
	return
}
func (awsSmStore *AwsSmStore) GetSmValues(ctx context.Context, secretName string) (secretValues map[string]string, err error) {
	logs.WithContext(ctx).Debug("GetSmValue - Start")

	if awsSmStore.client == nil {
		err = awsSmStore.Init(ctx)
		if err != nil {
			return
		}
	}

	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
	}

	result, err := awsSmStore.client.GetSecretValue(ctx, input)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	err = json.Unmarshal([]byte(*result.SecretString), &secretValues)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return
}
func (awsSmStore *AwsSmStore) GetSecretName() string {
	return awsSmStore.SmName
}
