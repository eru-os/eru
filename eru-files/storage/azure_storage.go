package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/segmentio/ksuid"
)

// const iv = "0123456789ABCDEF"

type AzureStorage struct {
	Storage
	Region         string `json:"region"`
	BucketName     string `json:"bucket_name" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	SubscriptionId string `json:"subscription_id" eru:"required"`
	TenantId       string `json:"tenant_id" eru:"required"`
	ClientId       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	ResourceGroup  string `json:"resource_group" eru:"required"`
	session        azcore.TokenCredential
}

func (azureStorage *AzureStorage) GetAttribute(attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "storage_name":
		return azureStorage.StorageName, nil
	case "storage_type":
		return azureStorage.StorageType, nil
	case "key_pair":
		return azureStorage.KeyPair, nil
	case "key_id":
		return azureStorage.KmsId, nil
	case "bucket_name":
		return azureStorage.BucketName, nil
	case "subscription_id":
		return azureStorage.SubscriptionId, nil
	case "resource_group":
		return azureStorage.ResourceGroup, nil
	case "region":
		return azureStorage.Region, nil
	case "authentication":
		return azureStorage.Authentication, nil
	case "client_id":
		return azureStorage.ClientId, nil
	case "client_secret":
		return azureStorage.ClientSecret, nil
	case "tenant_id":
		return azureStorage.TenantId, nil
	case "session":
		return azureStorage.session, nil
	default:
		return nil, errors.New("attribute not found")
	}
}

func (azureStorage *AzureStorage) ContainerExists(ctx context.Context, containerClient container.Client) (exists bool, err error) {
	logs.WithContext(ctx).Info("Conatiner Exists - Start")
	_, err = containerClient.GetProperties(ctx, nil)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (azureStorage *AzureStorage) CreateContainer(ctx context.Context, containerName string, containerClient container.Client) (container.Client, container.CreateResponse, error) {
	logs.WithContext(ctx).Info("Create Container - Start")

	createContainerResponse, err := containerClient.Create(ctx, nil)
	if err != nil {
		logs.WithContext(ctx).Info(fmt.Sprintf("Failed to create container: %s , Error: %s", containerName, err.Error()))
		return containerClient, container.CreateResponse{}, err
	}

	return containerClient, createContainerResponse, nil
}
func (azureStorage *AzureStorage) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("Init - Start")
	var cred azcore.TokenCredential
	armstorage.NewAccountsClient(azureStorage.SubscriptionId, cred, nil)

	if azureStorage.Authentication == AuthTypeSecret {
		cred, err = azidentity.NewClientSecretCredential(azureStorage.TenantId, azureStorage.ClientId, azureStorage.ClientSecret, nil)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("failed to create client secret credential: %s", err.Error()))
			return err
		}
	} else if azureStorage.Authentication == AuthTypeIAM {
		logs.WithContext(ctx).Info("connecting Azure storage with IAM role")
		cred, err = azidentity.NewDefaultAzureCredential(nil)
		if err == nil {
			// Test token acquisition to validate
			_, err = cred.GetToken(ctx, policy.TokenRequestOptions{
				Scopes: []string{"https://management.azure.com/.default"},
			})
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("failed to get token: %s", err.Error()))
				return err
			}
		}
	}
	azureStorage.session = cred
	return nil
}

func (azureStorage *AzureStorage) UploadFileB64(ctx context.Context, projectId string, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	_ = projectId
	logs.WithContext(ctx).Debug("UploadFileB64 - Start")



	if _, err := azureStorage.BucketExists(ctx); err != nil {
		logs.WithContext(ctx).Info("Bucket Not Exist")
		return "Bucket Not Exist", err
	}

	containerName := folderPath
	enc := ""
	finalFileName := ""
	docId = ksuid.New().String()

	bucketName := azureStorage.BucketName

	accountURL := fmt.Sprintf("https://%s.blob.core.windows.net/", bucketName)

	// Create credential using the service principal
	cred, err := azidentity.NewClientSecretCredential(azureStorage.TenantId, azureStorage.ClientId, azureStorage.ClientSecret, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create credential: %v", err))
		return docId, err
	}

	// Create blob service client
	serviceClient, err := azblob.NewClient(accountURL, cred, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create blob service client: %v", err))
		return docId, err
	}



	containerClient := serviceClient.ServiceClient().NewContainerClient(containerName)
	// check if container exists
	isContainerExist, err := azureStorage.ContainerExists(ctx, *containerClient)

	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to check container existence: %v", err))
		return docId, err
	}



	if isContainerExist {
		if azureStorage.EncryptFiles {
			enc = ".enc"
			var fileKey []byte
			finalFileName = fmt.Sprintf("%s_%s%s", docId, fileName, enc)
			file, fileKey, err = azureStorage.encrypt(ctx, file, keyName)
			if err != nil {

				logs.WithContext(ctx).Error(fmt.Sprintf("Failed to encrypt file: %v", err))



				return docId, err
			}
			file = append(file, []byte("___eru___")...)
			file = append(file, fileKey...)
		} else {
			finalFileName = fmt.Sprintf("%s_%s", docId, fileName)
		}

		blobClient := containerClient.NewBlockBlobClient(finalFileName)
		reader := bytes.NewReader(file)

		_, err = blobClient.UploadStream(ctx, reader, nil)
		if err != nil {

			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to upload file: %v", err))
			return docId, err

		
		}
		logs.WithContext(ctx).Info("File uploaded successfully")
		return docId, nil
	} else {

		logs.WithContext(ctx).Debug("Container does not exist, creating...")
		containerClient, createContainerResponse, err := azureStorage.CreateContainer(ctx, containerName, *containerClient)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create container: %v", err))
			return docId, err
		}


	
		if createContainerResponse.Date != nil {
			if azureStorage.EncryptFiles {
				enc = ".enc"
				var fileKey []byte
				finalFileName = fmt.Sprintf("%s_%s%s", docId, fileName, enc)
				file, fileKey, err = azureStorage.encrypt(ctx, file, keyName)
				if err != nil {

					logs.WithContext(ctx).Error(fmt.Sprintf("Failed to encrypt file: %v", err))


					return docId, err
				}
				file = append(file, []byte("___eru___")...)
				file = append(file, fileKey...)
			} else {
				finalFileName = fmt.Sprintf("%s_%s", docId, fileName)
			}

			blobClient := containerClient.NewBlockBlobClient(finalFileName)
			reader := bytes.NewReader(file)

			_, err = blobClient.UploadStream(ctx, reader, nil)
			if err != nil {

				logs.WithContext(ctx).Error(fmt.Sprintf("Failed to upload file: %v", err))
				return docId, err


			}
			logs.WithContext(ctx).Info("File uploaded successfully")
			return docId, nil
		}
		return docId, fmt.Errorf("failed to create container")
	}
}

func (azureStorage *AzureStorage) UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	_ = projectId
	logs.WithContext(ctx).Info("UploadFile - Start")
	if _, err := azureStorage.BucketExists(ctx); err != nil {
		logs.WithContext(ctx).Info("Bucket Not Exist")
		return "Bucket Not Exist", err
	}
	containerName := folderPath
	enc := ".enc"
	finalFileName := ""
	fileKey := []byte{}
	docId = ksuid.New().String()
	bucketName := azureStorage.BucketName

	cred, err := azidentity.NewClientSecretCredential(azureStorage.TenantId, azureStorage.ClientId, azureStorage.ClientSecret, nil)
	if err != nil {
		logs.WithContext(ctx).Info("Failed to get credential")
		return
	}
	// create storage client
	BucketUrl := fmt.Sprintf("https://%s.blob.core.windows.net/", bucketName)
	serviceClient, err := azblob.NewClient(BucketUrl, cred, nil)
	if err != nil {
		logs.WithContext(ctx).Info("Failed To create service client")
		return docId, err
	}

	containerClient := serviceClient.ServiceClient().NewContainerClient(containerName)
	// check if container exists
	isContainerExist, _ := azureStorage.ContainerExists(ctx, *containerClient)
	if isContainerExist {
		file, err := io.ReadAll(file)
		if err != nil {
			logs.WithContext(ctx).Info("Failed to read File content")
			return docId, err
		}
		if azureStorage.EncryptFiles {
			finalFileName = fmt.Sprintf("%s_%s%s", docId, header.Filename, enc)
			file, fileKey, err = azureStorage.encrypt(ctx, file, keyName)
			if err != nil {
				logs.WithContext(ctx).Info("Failed to encrypt file")
				return docId, err
			}
			file = append(file, []byte("___eru___")...)
			file = append(file, fileKey...)
		} else {
			finalFileName = fmt.Sprintf("%s_%s", docId, header.Filename)
		}
		blobClient := containerClient.NewBlockBlobClient(finalFileName)
		reader := bytes.NewReader(file)
		_, err = blobClient.UploadStream(ctx, reader, nil)
		if err != nil {
			slog.Info("Failed To Upload File")
			logs.WithContext(ctx).Info("Failed To Upload File")
			return docId, err
		} else {
			slog.Info("File Uploaded Successfully")
			logs.WithContext(ctx).Info("File Uploaded Successfully")
			return docId, err
		}
	} else {
		//Create new container and upload file
		logs.WithContext(ctx).Info("Container is not present, Container creation started...")
		containerClient, createContainerResponse, _ := azureStorage.CreateContainer(ctx, containerName, *containerClient)
		if createContainerResponse.Date != nil {
			file, err := io.ReadAll(file)
			if err != nil {
				slog.Info("Failed to read File content")
				logs.WithContext(ctx).Info("Failed to read File content")
				return docId, err
			}
			if azureStorage.EncryptFiles {
				finalFileName = fmt.Sprintf("%s_%s%s", docId, header.Filename, enc)
				file, fileKey, err = azureStorage.encrypt(ctx, file, keyName)
				if err != nil {
					logs.WithContext(ctx).Info("Failed to encrypt file")
					return docId, err
				}
				file = append(file, []byte("___eru___")...)
				file = append(file, fileKey...)
			} else {
				finalFileName = fmt.Sprintf("%s_%s", docId, header.Filename)
			}
			reader := bytes.NewReader(file)
			blobClient := containerClient.NewBlockBlobClient(finalFileName)
			_, err = blobClient.UploadStream(ctx, reader, nil)
			if err != nil {
				logs.WithContext(ctx).Info("Failed To Upload File")
				return docId, err
			} else {
				logs.WithContext(ctx).Info("File Uploaded Successfully")
				return docId, err
			}
		} else {
			logs.WithContext(ctx).Info(fmt.Sprintf("Container is not present %s , error: %s", containerName, "Failed to create new container"))
			return docId, fmt.Errorf("failed to create new container")
		}
	}

}
func (azureStorage *AzureStorage) DownloadFile(ctx context.Context, projectId string, folderPath string, fileName string, keyName eruaes.AesKey) (file []byte, err error) {
	_ = projectId
	logs.WithContext(ctx).Info("Downloading File - Start")
	if _, err := azureStorage.BucketExists(ctx); err != nil {
		logs.WithContext(ctx).Info("Bucket Not Exist")
		return []byte("Bucket Not Exist"), err
	}
	cred, err := azidentity.NewClientSecretCredential(azureStorage.TenantId, azureStorage.ClientId, azureStorage.ClientSecret, nil)
	if err != nil {
		logs.WithContext(ctx).Info("Failed to get credential")
		return
	}
	bucketName := azureStorage.BucketName
	BucketUrl := fmt.Sprintf("https://%s.blob.core.windows.net/", bucketName)
	serviceClient, err := azblob.NewClient(BucketUrl, cred, nil)
	if err != nil {
		logs.WithContext(ctx).Info("Failed to Create service client")
		return
	}
	containerName := folderPath
	blob_name := fileName
	containerClient := serviceClient.ServiceClient().NewContainerClient(containerName)
	isContainerExist, err := azureStorage.ContainerExists(ctx, *containerClient)
	if !isContainerExist {
		logs.WithContext(ctx).Info("Container is not present")
		return []byte("Container is not present"), err
	}
	if azureStorage.EncryptFiles {
		blob_name = fmt.Sprintf("%s.enc", fileName)
	}
	blb_client := containerClient.NewBlobClient(blob_name)
	_, err = blb_client.GetProperties(ctx, nil)
	if err != nil {
		logs.WithContext(ctx).Info(fmt.Sprintf("file '%s' does not exist in container '%s'", fileName, containerName))
		return nil, fmt.Errorf("blob does not exist: %w", err)
	}
	downloadResponse, err := blb_client.DownloadStream(ctx, nil)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		return
	}
	byteContainer, err := io.ReadAll(downloadResponse.Body)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		return
	}
	if azureStorage.EncryptFiles {
		var byteContainerKey []byte
		byteContainerSlice := bytes.Split(byteContainer, []byte("___eru___"))
		if len(byteContainerSlice) > 1 {
			byteContainerKey = byteContainerSlice[1]
		}
		byteContainer = byteContainerSlice[0]
		byteContainer, err = azureStorage.decrypt(ctx, byteContainer, byteContainerKey, keyName)
		if err != nil {
			logs.WithContext(ctx).Info("Failed while decrypting file")
			return
		} else {
			logs.WithContext(ctx).Info("File Decrypted successfully")
		}
	}
	logs.WithContext(ctx).Info("File Downloaded successfully")
	return byteContainer, err
}

func (azureStorage *AzureStorage) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	err := json.Unmarshal(*rj, &azureStorage)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (azureStorage *AzureStorage) DeleteStorage(ctx context.Context, projectId string, forceDelete bool, cloneStorage StorageI) (err error) {
	_ = projectId
	logs.WithContext(ctx).Info("DeleteStorage - Start")
	// Check if storage account exists
	exists, err := azureStorage.BucketExists(ctx)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to check storage account existence: %v", err))
		return err
	}
	if !exists {
		logs.WithContext(ctx).Info(fmt.Sprintf("Storage account %s does not exist in resource group %s", azureStorage.BucketName, azureStorage.ResourceGroup))
		return fmt.Errorf("storage account %s does not exist", azureStorage.BucketName)
	}

	// Create storage client
	storageClient, err := armstorage.NewAccountsClient(azureStorage.SubscriptionId, azureStorage.session, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create storage client: %v", err))
		return fmt.Errorf("failed to create storage client: %w", err)
	}

	// Delete the storage account
	logs.WithContext(ctx).Info(fmt.Sprintf("Starting deletion of storage account %s in resource group %s", azureStorage.BucketName, azureStorage.ResourceGroup))
	_, err = storageClient.Delete(ctx, azureStorage.ResourceGroup, azureStorage.BucketName, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to delete storage account %s: %v", azureStorage.BucketName, err))
		return fmt.Errorf("failed to delete storage account: %w", err)
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Storage account %s deleted successfully", azureStorage.BucketName))
	return nil
}

func (azureStorage *AzureStorage) CreateStorage(ctx context.Context, projectId string, cloneStorage StorageI, persist bool) (err error) {
	_ = projectId
	logs.WithContext(ctx).Info("creating Azure session")
	err = cloneStorage.Init(ctx)
	if err != nil {
		return
	}
	session, err := cloneStorage.GetAttribute("session")
	if err != nil {
		return
	}
	azureStorage.session = session.(azcore.TokenCredential)

	subscriptionId, err := cloneStorage.GetAttribute("subscription_id")
	if err != nil {
		return
	}
	resourceGroup, err := cloneStorage.GetAttribute("resource_group")
	if err != nil {
		return
	}
	if persist {
		be := false
		be, err = cloneStorage.BucketExists(ctx)
		if err != nil {
			return
		}
		logs.WithContext(ctx).Info(fmt.Sprint("be = ", be))
		if !be {
			bn, _ := cloneStorage.GetAttribute("bucket_name")
			logs.WithContext(ctx).Info(bn.(string))

			storageClient, err := armstorage.NewAccountsClient(subscriptionId.(string), azureStorage.session, nil)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("Failed to create storage client", err.Error()))
				return err
			}
			skuName := armstorage.SKUNameStandardLRS
			kind := armstorage.KindStorageV2
			accessTier := armstorage.AccessTierHot
			location := azureStorage.Region
			pollerResp, err := storageClient.BeginCreate(
				ctx,
				resourceGroup.(string),
				azureStorage.BucketName,
				armstorage.AccountCreateParameters{
					Location: &location,
					SKU: &armstorage.SKU{
						Name: &skuName,
					},
					Kind: &kind,
					Properties: &armstorage.AccountPropertiesCreateParameters{
						AccessTier: &accessTier,
					},
				},
				nil,
			)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("Failed to start storage account creation: %v", err))
				return err
			}
			_, err = pollerResp.PollUntilDone(ctx, nil)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("Failed to complete storage account creation: %v", err))
				return err
			}
		} else {
			logs.WithContext(ctx).Info("skipping container creation in Azure as it already exists")
		}
	}
	return
}

func (azureStorage *AzureStorage) BucketExists(ctx context.Context) (exists bool, err error) {
	if azureStorage.session == nil {
		logs.WithContext(ctx).Info("creating Azure session")
		err = azureStorage.Init(ctx)
		if err != nil {
			return false, err
		}
	}
	storageClient, err := armstorage.NewAccountsClient(azureStorage.SubscriptionId, azureStorage.session, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create storage client: %v", err))
		return false, err
	}
	_, err = storageClient.GetProperties(ctx, azureStorage.ResourceGroup, azureStorage.BucketName, nil)
	if err != nil {
		logs.WithContext(ctx).Info(fmt.Sprintf("Storage account %s does not exist: %v", azureStorage.BucketName, err))
		return false, nil
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Storage account %s exists", azureStorage.BucketName))
	return true, nil
}

func (azureStorage *AzureStorage) DeleteFile(ctx context.Context, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (status string, err error) {

	containerName := folderPath
	bucketName := azureStorage.BucketName
	BucketUrl := fmt.Sprintf("https://%s.blob.core.windows.net/", bucketName)
	cred, err := azidentity.NewClientSecretCredential(azureStorage.TenantId, azureStorage.ClientId, azureStorage.ClientSecret, nil)
	if err != nil {
		logs.WithContext(ctx).Info("Failed to get credential")
		return
	}
	serviceClient, _ := azblob.NewClient(BucketUrl, cred, nil)

	isContainerExist, _ := azureStorage.BucketExists(ctx)
	if isContainerExist {
		containerClient := serviceClient.ServiceClient().NewContainerClient(containerName)
		blobClient := containerClient.NewBlockBlobClient(fileName)
		_, err = blobClient.GetProperties(ctx, nil)
		if err != nil {
			_, err = blobClient.Delete(ctx, nil)
			if err != nil {
				logs.WithContext(ctx).Info("Failed To Delete File")
				return
			} else {
				logs.WithContext(ctx).Info("File Deleted Successfully")
			}
			return ("File Deleted Successfully"), err
		} else {
			logs.WithContext(ctx).Info("File is not present")
		}
	}
	logs.WithContext(ctx).Info("Container is not present")
	return ("File is not present in specified folder"), err

}

func (azureStorage AzureStorage) encrypt(ctx context.Context, byteContainer []byte, keyName eruaes.AesKey) (eByteContainer []byte, eByteContainerKey []byte, err error) {
	logs.WithContext(ctx).Info("encrypt - Start")
	if azureStorage.KeyPair != "" {
		byteContainer = eruaes.Pad(byteContainer, 16)
		eByteContainer, err = eruaes.EncryptCBC(ctx, byteContainer, keyName.Key, keyName.Vector)
		if err != nil {
			return
		}
	} else if azureStorage.KmsKey != nil {
		/*aKey := eruaes.AesKey{}
		aKey, err = eruaes.GenerateKey(ctx, 16)
		if err != nil {
			return
		}

		byteContainer = eruaes.Pad(byteContainer, 16)
		eByteContainer, err = eruaes.EncryptCBC(ctx, byteContainer, aKey.Key, []byte(iv))
		if err != nil {
			return
		}

		 eByteContainerKey, err = azureStorage.KmsKey.Encrypt(ctx, aKey.Key)
		if err != nil {
			return
		} */
	} else {
		err = errors.New("encryption key not found")
		return
	}
	return

}

func (azureStorage AzureStorage) decrypt(ctx context.Context, eByteContainer []byte, byteContainerKey []byte, keyName eruaes.AesKey) (byteContainer []byte, err error) {
	if azureStorage.KeyPair != "" {
		byteContainer, err = eruaes.DecryptCBC(ctx, eByteContainer, keyName.Key, keyName.Vector)
		if err != nil {
			return
		}
		byteContainer, err = eruaes.Unpad(byteContainer)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	} else if azureStorage.KmsKey != nil {
		byteContainerKey, err = azureStorage.KmsKey.Decrypt(ctx, byteContainerKey)
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
		err = errors.New("decryption key not found")
		return
	}
	return
}
