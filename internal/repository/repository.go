package repository

type Repository interface {
	ProfileRepository
	ExecutionRepository
	NotificationProviderRepository
	RetentionPolicyRepository
	StorageProviderRepository
}
