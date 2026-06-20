//go:build integration

package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/domain"
	infrakyc "github.com/kodiahbertrand/pillow/internal/infrastructure/kycprovider"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"github.com/kodiahbertrand/pillow/internal/kyc"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
	"gorm.io/gorm"
)

const webhookSecret = "integration-webhook-secret"

func TestIntegration_ProviderWebhook(t *testing.T) {
	ctx := context.Background()
	db, cleanup := testhelpers.NewPostgresContainer(t, ctx)
	defer cleanup()

	svc, repo, _ := newKYCService(db, fixedSanctions{result: approvedProviderResult()})
	verifier := infrakyc.NewWebhookVerifier(webhookSecret)
	handler := kyc.NewHandler(svc, verifier)

	app := fiber.New(fiber.Config{ErrorHandler: kycErrorHandler})
	app.Post("/kyc/webhooks/provider", handler.HandleProviderWebhook)

	t.Run("signed verdict transitions case and persists check", func(t *testing.T) {
		userID := seedUserWithProfile(t, ctx, db, repo, "webhook-approve@example.com")
		caseID := seedCase(t, ctx, repo, userID, domain.CheckDocument)

		body := webhookBody("evt-approve-1", caseID, "document", "approved")
		resp := postWebhook(t, app, body, sign(body))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		if got := caseStatus(t, ctx, repo, caseID); got != domain.StatusVerified {
			t.Errorf("case status = %q, want verified", got)
		}
		if n := countChecks(ctx, db, "evt-approve-1"); n != 1 {
			t.Errorf("check rows = %d, want 1", n)
		}
	})

	t.Run("duplicate event id produces exactly one state change", func(t *testing.T) {
		userID := seedUserWithProfile(t, ctx, db, repo, "webhook-dup@example.com")
		caseID := seedCase(t, ctx, repo, userID, domain.CheckDocument)

		body := webhookBody("evt-dup-1", caseID, "document", "approved")
		for i := 0; i < 2; i++ {
			resp := postWebhook(t, app, body, sign(body))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("delivery %d: status = %d, want 200", i+1, resp.StatusCode)
			}
		}

		if n := countChecks(ctx, db, "evt-dup-1"); n != 1 {
			t.Errorf("check rows after duplicate delivery = %d, want exactly 1", n)
		}
		if got := caseStatus(t, ctx, repo, caseID); got != domain.StatusVerified {
			t.Errorf("case status = %q, want verified", got)
		}
	})

	t.Run("bad signature is rejected with no state change", func(t *testing.T) {
		userID := seedUserWithProfile(t, ctx, db, repo, "webhook-badsig@example.com")
		caseID := seedCase(t, ctx, repo, userID, domain.CheckDocument)

		body := webhookBody("evt-badsig-1", caseID, "document", "approved")
		resp := postWebhook(t, app, body, "deadbeef")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}

		if got := caseStatus(t, ctx, repo, caseID); got != domain.StatusPending {
			t.Errorf("case status = %q, want pending (unchanged)", got)
		}
		if n := countChecks(ctx, db, "evt-badsig-1"); n != 0 {
			t.Errorf("check rows = %d, want 0", n)
		}
	})
}

func seedUserWithProfile(t *testing.T, ctx context.Context, db *gorm.DB, repo domain.KYCRepository, email string) string {
	t.Helper()
	user := &pgmodels.UserModel{Email: email, DisplayName: "Webhook User"}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := repo.CreateProfile(ctx, &domain.KYCProfile{
		UserID: user.ID,
		Role:   domain.RoleBuyer,
		Tier:   domain.TierAnonymous,
		Status: domain.StatusPending,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return user.ID
}

func seedCase(t *testing.T, ctx context.Context, repo domain.KYCRepository, userID string, checkType domain.CheckType) string {
	t.Helper()
	vc := &domain.VerificationCase{UserID: userID, Type: checkType, Status: domain.StatusPending}
	if err := repo.CreateCase(ctx, vc); err != nil {
		t.Fatalf("seed case: %v", err)
	}
	return vc.ID
}

func caseStatus(t *testing.T, ctx context.Context, repo domain.KYCRepository, caseID string) domain.VerificationStatus {
	t.Helper()
	vc, err := repo.FindCaseByID(ctx, caseID)
	if err != nil {
		t.Fatalf("find case: %v", err)
	}
	return vc.Status
}

func countChecks(ctx context.Context, db *gorm.DB, eventID string) int64 {
	var n int64
	db.WithContext(ctx).Model(&pgmodels.KYCCheckModel{}).Where("provider_event_id = ?", eventID).Count(&n)
	return n
}

func webhookBody(eventID, caseID, checkType, verdict string) string {
	return fmt.Sprintf(`{"event_id":%q,"case_id":%q,"check_type":%q,"verdict":%q,"risk_score":5,"raw":{"source":"integration"}}`,
		eventID, caseID, checkType, verdict)
}

func sign(body string) string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func postWebhook(t *testing.T, app *fiber.App, body, signature string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-KYC-Signature", signature)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("webhook request: %v", err)
	}
	return resp
}
