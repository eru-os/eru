package kms

type GcpKmsStore struct {
	KmsStore
	KmsName string `json:"kms_name" eru:"required"`
}
