package repository

type Repository interface {
	ProfileRepository
	ExecutionRepository
	ExecutionStorageRepository
	RestoreRepository
	NotificationProviderRepository
	RetentionPolicyRepository
	StorageProviderRepository
}
