package kyc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/pkg/metrics"
	"github.com/kodiahbertrand/pillow/pkg/resilience"
)

func traceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type CaseReconciler struct {
	repo     domain.KYCRepository
	audit    domain.AuditRepository
	notifier domain.Notifier
	sla      time.Duration
}

func NewCaseReconciler(repo domain.KYCRepository, audit domain.AuditRepository, notifier domain.Notifier, sla time.Duration) *CaseReconciler {
	return &CaseReconciler{repo: repo, audit: audit, notifier: notifier, sla: sla}
}

func (r *CaseReconciler) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *CaseReconciler) RunOnce(ctx context.Context) {
	tid := traceID()
	slog.Info("kyc: reconciler: run once", "trace_id", tid)
	cutoff := time.Now().Add(-r.sla)
	cases, err := r.repo.ListCasesStuckInReview(ctx, cutoff, 100)
	if err != nil {
		slog.Error("kyc: reconciler: list stuck cases failed", "error", err, "trace_id", tid)
		return
	}
	for _, vc := range cases {
		evt := &domain.AuditEvent{
			UserID:    vc.UserID,
			EventType: "kyc_case_stuck_in_review",
			Metadata:  map[string]any{"case_id": vc.ID, "stuck_since": vc.UpdatedAt},
		}
		if appendErr := r.audit.Append(ctx, evt); appendErr != nil {
			slog.Error("kyc: reconciler: audit append failed", "error", appendErr, "case_id", vc.ID)
		}
		slog.Warn("kyc: case stuck in review", "case_id", vc.ID, "user_id", vc.UserID, "updated_at", vc.UpdatedAt)
		_ = r.notifier.Notify(ctx, domain.Notification{
			UserID:    vc.UserID,
			EventType: "kyc_case_stuck_in_review",
			CaseID:    vc.ID,
			Status:    vc.Status,
		})
	}
}

type SanctionsRescreener struct {
	repo      domain.KYCRepository
	audit     domain.AuditRepository
	sanctions domain.SanctionsScreener
	service   domain.KYCService
	minTier   domain.Tier
}

func NewSanctionsRescreener(repo domain.KYCRepository, audit domain.AuditRepository, sanctions domain.SanctionsScreener, service domain.KYCService) *SanctionsRescreener {
	return &SanctionsRescreener{
		repo:      repo,
		audit:     audit,
		sanctions: sanctions,
		service:   service,
		minTier:   domain.TierVerified,
	}
}

func (s *SanctionsRescreener) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *SanctionsRescreener) RunOnce(ctx context.Context) {
	tid := traceID()
	slog.Info("kyc: rescreener: run once", "trace_id", tid)
	const batchSize = 50
	cursor := ""
	for {
		profiles, err := s.repo.ListProfilesForRescreen(ctx, s.minTier, batchSize, cursor)
		if err != nil {
			slog.Error("kyc: rescreener: list profiles failed", "error", err, "trace_id", tid)
			return
		}
		if len(profiles) == 0 {
			return
		}
		for _, p := range profiles {
			s.rescreen(ctx, p, tid)
		}
		cursor = profiles[len(profiles)-1].UserID
		if len(profiles) < batchSize {
			return
		}
	}
}

func (s *SanctionsRescreener) rescreen(ctx context.Context, p domain.KYCProfile, tid string) {
	result, err := s.sanctions.Screen(ctx, domain.SanctionsRequest{UserID: p.UserID})
	if err != nil {
		if errors.Is(err, resilience.ErrCircuitOpen) {
			slog.Warn("kyc: rescreener: circuit open, skipping", "user_id", p.UserID, "trace_id", tid)
			return
		}
		slog.Error("kyc: rescreener: screen failed", "error", err, "user_id", p.UserID, "trace_id", tid)
		return
	}

	evt := &domain.AuditEvent{
		UserID:    p.UserID,
		EventType: "kyc_sanctions_rescreen",
		Tier:      p.Tier,
		Metadata:  map[string]any{"verdict": string(result.Verdict)},
	}
	if appendErr := s.audit.Append(ctx, evt); appendErr != nil {
		slog.Error("kyc: rescreener: audit append failed", "error", appendErr, "user_id", p.UserID)
	}

	if result.Verdict == domain.VerdictRejected {
		s.enforce(ctx, p, result)
	}
}

func (s *SanctionsRescreener) enforce(ctx context.Context, p domain.KYCProfile, result *domain.ProviderCheckResult) {
	if result.ProviderEventID == "" {
		slog.Warn("kyc: rescreener: sanctions hit without provider event id, skipping enforcement", "user_id", p.UserID)
		return
	}
	if _, err := s.repo.FindCheckByProviderEventID(ctx, result.ProviderEventID); err == nil {
		return
	}

	verificationCase := &domain.VerificationCase{
		UserID:    p.UserID,
		Type:      domain.CheckSanctions,
		Status:    domain.StatusPending,
		RiskScore: result.RiskScore,
	}
	if err := s.repo.CreateCase(ctx, verificationCase); err != nil {
		slog.Error("kyc: rescreener: open sanctions case failed", "error", err, "user_id", p.UserID)
		return
	}

	if err := s.service.ApplyVerdict(ctx, domain.VerdictResult{
		ProviderEventID: result.ProviderEventID,
		CaseID:          verificationCase.ID,
		Type:            domain.CheckSanctions,
		Verdict:         result.Verdict,
		RiskScore:       result.RiskScore,
		RawPayload:      result.RawPayload,
	}); err != nil {
		slog.Error("kyc: rescreener: apply sanctions verdict failed", "error", err, "user_id", p.UserID, "case_id", verificationCase.ID)
		return
	}
	slog.Warn("kyc: sanctions hit on rescreen", "user_id", p.UserID, "case_id", verificationCase.ID)
}

type LicenseExpiryChecker struct {
	repo     domain.KYCRepository
	audit    domain.AuditRepository
	notifier domain.Notifier
	window   time.Duration
}

func NewLicenseExpiryChecker(repo domain.KYCRepository, audit domain.AuditRepository, notifier domain.Notifier, window time.Duration) *LicenseExpiryChecker {
	return &LicenseExpiryChecker{repo: repo, audit: audit, notifier: notifier, window: window}
}

func (l *LicenseExpiryChecker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (l *LicenseExpiryChecker) RunOnce(ctx context.Context) {
	tid := traceID()
	slog.Info("kyc: license expiry: run once", "trace_id", tid)
	deadline := time.Now().Add(l.window)
	checks, err := l.repo.ListChecksExpiringBefore(ctx, deadline, 100)
	if err != nil {
		slog.Error("kyc: license expiry: list checks failed", "error", err, "trace_id", tid)
		return
	}
	for _, c := range checks {
		verificationCase, err := l.repo.FindCaseByID(ctx, c.CaseID)
		if err != nil {
			slog.Error("kyc: license expiry: find case failed", "error", err, "check_id", c.ID, "case_id", c.CaseID)
			continue
		}
		evt := &domain.AuditEvent{
			UserID:    verificationCase.UserID,
			EventType: "kyc_license_expiring",
			Metadata:  map[string]any{"check_id": c.ID, "case_id": c.CaseID, "expires_at": c.ExpiresAt},
		}
		if appendErr := l.audit.Append(ctx, evt); appendErr != nil {
			slog.Error("kyc: license expiry: audit append failed", "error", appendErr, "check_id", c.ID)
		}
		_ = l.notifier.Notify(ctx, domain.Notification{
			UserID:    verificationCase.UserID,
			EventType: "kyc_license_expiring",
			CaseID:    c.CaseID,
			Status:    c.Status,
		})
		slog.Info("kyc: license expiring soon", "check_id", c.ID, "user_id", verificationCase.UserID, "expires_at", c.ExpiresAt)
	}
}

type VaultPurger struct {
	vault domain.DocumentVault
}

func NewVaultPurger(vault domain.DocumentVault) *VaultPurger {
	return &VaultPurger{vault: vault}
}

func (v *VaultPurger) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			v.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (v *VaultPurger) RunOnce(ctx context.Context) {
	tid := traceID()
	slog.Info("kyc: vault purger: run once", "trace_id", tid)
	count, err := v.vault.PurgeExpired(ctx)
	if err != nil {
		slog.Error("kyc: vault purger: purge failed", "error", err, "trace_id", tid)
		return
	}
	if count > 0 {
		slog.Info("kyc: vault purger: purged expired documents", "count", count, "trace_id", tid)
	}
}

type QueueConsumer struct {
	repo         domain.KYCRepository
	audit        domain.AuditRepository
	service      domain.KYCService
	identity     domain.IdentityVerifier
	sanctions    domain.SanctionsScreener
	ownership    domain.OwnershipVerifier
	license      domain.LicenseVerifier
	business     domain.BusinessVerifier
	notifier     domain.Notifier
	queue        domain.VerificationQueue
	cache        domain.TierCache
	metrics      metrics.Metrics
	manualReview bool
}

func NewQueueConsumer(
	repo domain.KYCRepository,
	audit domain.AuditRepository,
	service domain.KYCService,
	identity domain.IdentityVerifier,
	sanctions domain.SanctionsScreener,
	ownership domain.OwnershipVerifier,
	license domain.LicenseVerifier,
	business domain.BusinessVerifier,
	notifier domain.Notifier,
	queue domain.VerificationQueue,
	cache domain.TierCache,
	met metrics.Metrics,
	manualReview bool,
) *QueueConsumer {
	return &QueueConsumer{
		repo:         repo,
		audit:        audit,
		service:      service,
		identity:     identity,
		sanctions:    sanctions,
		ownership:    ownership,
		license:      license,
		business:     business,
		notifier:     notifier,
		queue:        queue,
		cache:        cache,
		metrics:      met,
		manualReview: manualReview,
	}
}

func (c *QueueConsumer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (c *QueueConsumer) RunOnce(ctx context.Context) {
	tid := traceID()
	slog.Info("kyc: queue consumer: run once", "trace_id", tid)
	if depth, err := c.queue.Len(ctx); err == nil {
		c.metrics.SetQueueDepth(depth)
	}
	job, err := c.queue.Dequeue(ctx, 5*time.Second)
	if err != nil {
		slog.Warn("kyc: queue consumer: dequeue failed", "error", err, "trace_id", tid)
		return
	}
	if job == nil {
		return
	}
	if err := c.processJob(ctx, job); err != nil {
		slog.Error("kyc: queue consumer: process job failed", "error", err, "trace_id", tid, "case_id", job.CaseID, "claim_id", job.ClaimID, "type", job.Type)
	}
}

func (c *QueueConsumer) timedProviderCall(provider string, fn func() error) error {
	start := time.Now()
	err := fn()
	c.metrics.RecordVendorLatency(provider, time.Since(start))
	if errors.Is(err, resilience.ErrCircuitOpen) {
		c.metrics.IncCircuitOpen(provider)
	}
	return err
}

func (c *QueueConsumer) processJob(ctx context.Context, job *domain.VerificationJob) error {
	if c.manualReview {
		return c.parkForManualReview(ctx, job)
	}
	switch job.Type {
	case domain.CheckLicense:
		var result *domain.ProviderCheckResult
		if err := c.timedProviderCall("license", func() (innerErr error) {
			result, innerErr = c.license.VerifyLicense(ctx, domain.LicenseVerificationRequest{
				UserID:        job.UserID,
				LicenseNumber: job.LicenseNumber,
				Jurisdiction:  job.Jurisdiction,
			})
			return
		}); err != nil {
			return fmt.Errorf("verify license: %w", err)
		}
		return c.service.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: result.ProviderEventID,
			CaseID:          job.CaseID,
			Type:            job.Type,
			Verdict:         result.Verdict,
			RiskScore:       result.RiskScore,
			RawPayload:      result.RawPayload,
		})

	case domain.CheckKYB:
		var result *domain.ProviderCheckResult
		if err := c.timedProviderCall("business", func() (innerErr error) {
			result, innerErr = c.business.VerifyBusiness(ctx, domain.BusinessVerificationRequest{
				UserID:             job.UserID,
				BusinessName:       job.BusinessName,
				RegistrationNumber: job.RegistrationNumber,
			})
			return
		}); err != nil {
			return fmt.Errorf("verify business: %w", err)
		}
		return c.service.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: result.ProviderEventID,
			CaseID:          job.CaseID,
			Type:            job.Type,
			Verdict:         result.Verdict,
			RiskScore:       result.RiskScore,
			RawPayload:      result.RawPayload,
		})

	case domain.CheckOwnership:
		var result *domain.ProviderCheckResult
		if err := c.timedProviderCall("ownership", func() (innerErr error) {
			result, innerErr = c.ownership.VerifyOwnership(ctx, domain.OwnershipVerificationRequest{
				UserID:          job.UserID,
				PropertyAddress: job.PropertyAddress,
				ClaimantName:    job.ClaimantName,
			})
			return
		}); err != nil {
			return fmt.Errorf("verify ownership: %w", err)
		}

		claim, findErr := c.repo.FindOwnershipClaimByID(ctx, job.CaseID)
		if findErr != nil {
			return fmt.Errorf("find ownership claim: %w", findErr)
		}

		switch result.Verdict {
		case domain.VerdictApproved:
			claim.Status = domain.StatusVerified
			claim.Method = domain.OwnershipMethodPublicRecord
		case domain.VerdictRejected:
			claim.Status = domain.StatusRejected
			claim.Method = domain.OwnershipMethodDocument
		default:
			claim.Status = domain.StatusInReview
			claim.Method = domain.OwnershipMethodDocument
		}

		if err := c.repo.UpdateOwnershipClaim(ctx, claim); err != nil {
			return fmt.Errorf("update ownership claim: %w", err)
		}

		if appendErr := c.audit.Append(ctx, &domain.AuditEvent{
			UserID:    job.UserID,
			EventType: "ownership_claim_completed",
			Metadata:  map[string]any{"claim_id": claim.ID, "verdict": string(result.Verdict), "status": string(claim.Status)},
		}); appendErr != nil {
			slog.Error("kyc: queue consumer: ownership audit append failed", "error", appendErr, "claim_id", claim.ID)
		}

		if claim.Status == domain.StatusVerified {
			profile, profileErr := c.repo.FindProfileByUserID(ctx, job.UserID)
			if profileErr == nil {
				profile.GrantQualification(domain.QualificationOwnership)
				_ = c.repo.UpdateProfile(ctx, profile)
				if c.cache != nil {
					_ = c.cache.Del(ctx, job.UserID)
				}
			}
		}

		_ = c.notifier.Notify(ctx, domain.Notification{
			UserID:    job.UserID,
			EventType: "ownership_claim_completed",
			CaseID:    claim.ID,
			Status:    claim.Status,
		})
		return nil

	case domain.CheckDocument:
		var result *domain.ProviderCheckResult
		if err := c.timedProviderCall("document", func() (innerErr error) {
			result, innerErr = c.identity.VerifyDocument(ctx, domain.DocumentVerificationRequest{
				UserID:            job.UserID,
				DocumentReference: job.DocumentReference,
			})
			return
		}); err != nil {
			return fmt.Errorf("verify document: %w", err)
		}
		return c.service.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: result.ProviderEventID,
			CaseID:          job.CaseID,
			Type:            job.Type,
			Verdict:         result.Verdict,
			RiskScore:       result.RiskScore,
			RawPayload:      result.RawPayload,
		})

	case domain.CheckPayoutAML:
		var result *domain.ProviderCheckResult
		if err := c.timedProviderCall("payout_aml", func() (innerErr error) {
			result, innerErr = c.sanctions.Screen(ctx, domain.SanctionsRequest{
				UserID:   job.UserID,
				FullName: "",
			})
			return
		}); err != nil {
			if errors.Is(err, resilience.ErrCircuitOpen) {
				slog.Warn("kyc: queue consumer: payout aml circuit open, skipping", "case_id", job.CaseID)
				return nil
			}
			return fmt.Errorf("screen payout aml: %w", err)
		}
		return c.service.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: result.ProviderEventID,
			CaseID:          job.CaseID,
			Type:            job.Type,
			Verdict:         result.Verdict,
			RiskScore:       result.RiskScore,
			RawPayload:      result.RawPayload,
		})

	case domain.CheckSanctions:
		var result *domain.ProviderCheckResult
		if err := c.timedProviderCall("sanctions", func() (innerErr error) {
			result, innerErr = c.sanctions.Screen(ctx, domain.SanctionsRequest{
				UserID:   job.UserID,
				FullName: "",
			})
			return
		}); err != nil {
			if errors.Is(err, resilience.ErrCircuitOpen) {
				slog.Warn("kyc: queue consumer: sanctions circuit open, skipping", "case_id", job.CaseID)
				return nil
			}
			return fmt.Errorf("screen sanctions: %w", err)
		}
		return c.service.ApplyVerdict(ctx, domain.VerdictResult{
			ProviderEventID: result.ProviderEventID,
			CaseID:          job.CaseID,
			Type:            job.Type,
			Verdict:         result.Verdict,
			RiskScore:       result.RiskScore,
			RawPayload:      result.RawPayload,
		})

	default:
		slog.Warn("kyc: queue consumer: unknown job type", "type", job.Type)
		return nil
	}
}

func (c *QueueConsumer) parkForManualReview(ctx context.Context, job *domain.VerificationJob) error {
	if job.Type == domain.CheckOwnership {
		return c.parkClaimForManualReview(ctx, job)
	}

	verificationCase, err := c.repo.FindCaseByID(ctx, job.CaseID)
	if err != nil {
		return fmt.Errorf("manual review: find case: %w", err)
	}

	check := &domain.Check{
		CaseID:     job.CaseID,
		Type:       job.Type,
		Status:     domain.StatusInReview,
		Verdict:    domain.VerdictReview,
		RawPayload: manualReviewPayload(job),
	}
	if err := c.repo.CreateCheck(ctx, check); err != nil {
		return fmt.Errorf("manual review: create check: %w", err)
	}

	if verificationCase.Status == domain.StatusPending {
		next, transitionErr := domain.NextStatus(verificationCase.Status, domain.EventSubmit)
		if transitionErr == nil {
			verificationCase.Status = next
			if err := c.repo.UpdateCase(ctx, verificationCase); err != nil {
				return fmt.Errorf("manual review: update case: %w", err)
			}
		}
	}

	if appendErr := c.audit.Append(ctx, &domain.AuditEvent{
		UserID:    job.UserID,
		EventType: "kyc_case_awaiting_manual_review",
		Metadata:  map[string]any{"case_id": job.CaseID, "type": string(job.Type)},
	}); appendErr != nil {
		slog.Error("kyc: manual review: audit append failed", "error", appendErr, "case_id", job.CaseID)
	}
	slog.Info("kyc: case parked for manual review", "case_id", job.CaseID, "type", job.Type)
	return nil
}

func (c *QueueConsumer) parkClaimForManualReview(ctx context.Context, job *domain.VerificationJob) error {
	claim, err := c.repo.FindOwnershipClaimByID(ctx, job.CaseID)
	if err != nil {
		return fmt.Errorf("manual review: find ownership claim: %w", err)
	}

	claim.Status = domain.StatusInReview
	claim.Method = domain.OwnershipMethodManualReview
	if err := c.repo.UpdateOwnershipClaim(ctx, claim); err != nil {
		return fmt.Errorf("manual review: update ownership claim: %w", err)
	}

	if appendErr := c.audit.Append(ctx, &domain.AuditEvent{
		UserID:    job.UserID,
		EventType: "ownership_claim_awaiting_manual_review",
		Metadata:  map[string]any{"claim_id": claim.ID},
	}); appendErr != nil {
		slog.Error("kyc: manual review: audit append failed", "error", appendErr, "claim_id", claim.ID)
	}
	slog.Info("kyc: ownership claim parked for manual review", "claim_id", claim.ID)
	return nil
}

func manualReviewPayload(job *domain.VerificationJob) map[string]any {
	payload := map[string]any{"source": "manual_review_queue"}
	if job.DocumentReference != "" {
		payload["document_reference"] = string(job.DocumentReference)
	}
	if job.LicenseNumber != "" {
		payload["license_number"] = job.LicenseNumber
	}
	if job.Jurisdiction != "" {
		payload["jurisdiction"] = job.Jurisdiction
	}
	if job.BusinessName != "" {
		payload["business_name"] = job.BusinessName
	}
	if job.RegistrationNumber != "" {
		payload["registration_number"] = job.RegistrationNumber
	}
	return payload
}
