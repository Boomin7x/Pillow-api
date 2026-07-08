package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kodiahbertrand/pillow/internal/config"
	infrapostgres "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	infraredis "github.com/kodiahbertrand/pillow/internal/infrastructure/redis"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
)

type Worker struct {
	reconciler     *kyc.CaseReconciler
	rescreener     *kyc.SanctionsRescreener
	licenseChecker *kyc.LicenseExpiryChecker
	vaultPurger    *kyc.VaultPurger
	queueConsumer  *kyc.QueueConsumer
	cfg            *config.Config
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

func NewWorker(cfg *config.Config) (*Worker, error) {
	db, err := infrapostgres.NewDB(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("worker: init postgres: %w", err)
	}

	rdb, err := infraredis.NewClient(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("worker: init redis: %w", err)
	}

	components, err := buildKYC(cfg, db, rdb)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		reconciler:     kyc.NewCaseReconciler(components.repo, components.audit, components.notifier, cfg.Worker.ReconcileSLA),
		rescreener:     kyc.NewSanctionsRescreener(components.repo, components.audit, components.sanctions, components.service),
		licenseChecker: kyc.NewLicenseExpiryChecker(components.repo, components.audit, components.notifier, 7*24*time.Hour),
		vaultPurger:    kyc.NewVaultPurger(components.vault),
		queueConsumer: kyc.NewQueueConsumer(
			components.repo, components.audit, components.service,
			components.identity, components.sanctions,
			components.ownership, components.license,
			components.business, components.notifier,
			components.queue, components.cache,
			metrics.NewNoop(),
			cfg.KYC.ReviewMode == config.KYCReviewModeManual,
		),
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (w *Worker) Run() {
	if !w.cfg.Worker.Enabled {
		return
	}

	w.wg.Add(5)
	go func() {
		defer w.wg.Done()
		w.reconciler.Run(w.ctx, w.cfg.Worker.ReconcileInterval)
	}()
	go func() {
		defer w.wg.Done()
		w.rescreener.Run(w.ctx, w.cfg.Worker.RescreenInterval)
	}()
	go func() {
		defer w.wg.Done()
		w.licenseChecker.Run(w.ctx, w.cfg.Worker.LicenseExpiryInterval)
	}()
	go func() {
		defer w.wg.Done()
		w.vaultPurger.Run(w.ctx, w.cfg.Worker.VaultPurgeInterval)
	}()
	go func() {
		defer w.wg.Done()
		w.queueConsumer.Run(w.ctx, 2*time.Second)
	}()
}

func (w *Worker) Shutdown() {
	w.cancel()
	w.wg.Wait()
}
