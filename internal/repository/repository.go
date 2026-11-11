package repository

type Repository interface {
	ProfileRepository
	ExecutionRepository
	ExecutionStorageRepository
	RestoreRepository
	UserRepository
	APIKeyRepository
	AuditRepository
	NotificationProviderRepository
	RetentionPolicyRepository
	StorageProviderRepository
}
