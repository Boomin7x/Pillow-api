package app

import (
	"fmt"

	"github.com/kodiahbertrand/pillow/internal/config"
	"github.com/kodiahbertrand/pillow/internal/domain"
	infrabusiness "github.com/kodiahbertrand/pillow/internal/infrastructure/businessverify"
	infrakyccache "github.com/kodiahbertrand/pillow/internal/infrastructure/kyccache"
	infrakyc "github.com/kodiahbertrand/pillow/internal/infrastructure/kycprovider"
	infralicense "github.com/kodiahbertrand/pillow/internal/infrastructure/license"
	infranotify "github.com/kodiahbertrand/pillow/internal/infrastructure/notify"
	infraownership "github.com/kodiahbertrand/pillow/internal/infrastructure/ownership"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	infrastorage "github.com/kodiahbertrand/pillow/internal/infrastructure/storage"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type kycComponents struct {
	repo            domain.KYCRepository
	audit           domain.AuditRepository
	service         domain.KYCService
	identity        domain.IdentityVerifier
	sanctions       domain.SanctionsScreener
	ownership       domain.OwnershipVerifier
	license         domain.LicenseVerifier
	business        domain.BusinessVerifier
	vault           domain.DocumentVault
	notifier        domain.Notifier
	webhookVerifier domain.WebhookVerifier
	queue           domain.VerificationQueue
	cache           domain.TierCache
}

func buildKYC(cfg *config.Config, db *gorm.DB, rdb *redis.Client) (*kycComponents, error) {
	provider := infrakyc.NewProvider(cfg.KYC.IdentityProvider)
	ownershipVerifier := infraownership.NewVerifier(cfg.KYC.Ownership)
	licenseVerifier := infralicense.NewVerifier(cfg.KYC.License)
	businessVerifier := infrabusiness.NewVerifier(cfg.KYC.Business)

	vault, err := infrastorage.NewDocumentVault(cfg.KYC.Vault)
	if err != nil {
		return nil, fmt.Errorf("app: init document vault: %w", err)
	}

	repo := kyc.NewRepository(db)
	audit := kyc.NewAuditRepository(db)
	notifier := infranotify.New()

	var cache domain.TierCache
	if rdb != nil {
		cache = infrakyccache.NewTierCache(rdb, cfg.KYC.TierCacheTTL)
	}

	var queue domain.VerificationQueue
	if rdb != nil {
		queue = infraredis.NewVerificationQueue(rdb)
	}

	metric := metrics.NewNoop()

	service := kyc.NewService(repo, audit, provider, provider, ownershipVerifier, licenseVerifier, businessVerifier, vault, notifier, cache, queue, metric)

	return &kycComponents{
		repo:            repo,
		audit:           audit,
		service:         service,
		identity:        provider,
		sanctions:       provider,
		ownership:       ownershipVerifier,
		license:         licenseVerifier,
		business:        businessVerifier,
		vault:           vault,
		notifier:        notifier,
		webhookVerifier: infrakyc.NewWebhookVerifier(cfg.KYC.WebhookSecret),
		queue:           queue,
		cache:           cache,
	}, nil
}
