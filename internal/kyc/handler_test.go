package kyc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/kyc"
)

var fixedTime = time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

type mockWebhookVerifier struct {
	verifyFn func(payload []byte, sig string) error
}

func (m *mockWebhookVerifier) Verify(payload []byte, sig string) error {
	if m.verifyFn != nil {
		return m.verifyFn(payload, sig)
	}
	return nil
}

type mockKYCService struct {
	getProfileFn                func(ctx context.Context, userID string) (*domain.KYCProfile, error)
	evaluateAccessFn            func(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error)
	startVerificationFn         func(ctx context.Context, input domain.StartVerificationInput) (*domain.VerificationCase, error)
	submitDocumentFn            func(ctx context.Context, input domain.SubmitDocumentInput) error
	uploadDocumentFn            func(ctx context.Context, input domain.UploadDocumentInput) (*domain.VerificationCase, error)
	getCaseFn                   func(ctx context.Context, userID, caseID string) (*domain.VerificationCase, error)
	applyVerdictFn              func(ctx context.Context, result domain.VerdictResult) error
	startOwnershipClaimFn       func(ctx context.Context, input domain.StartOwnershipClaimInput) (*domain.OwnershipClaim, error)
	submitLicenseFn             func(ctx context.Context, input domain.SubmitLicenseInput) (*domain.VerificationCase, error)
	startBusinessVerificationFn func(ctx context.Context, input domain.StartBusinessVerificationInput) (*domain.VerificationCase, error)
	deleteProfileFn             func(ctx context.Context, userID string) error
}

func (m *mockKYCService) GetProfile(ctx context.Context, userID string) (*domain.KYCProfile, error) {
	if m.getProfileFn != nil {
		return m.getProfileFn(ctx, userID)
	}
	return &domain.KYCProfile{UserID: userID, Role: domain.RoleBuyer, Tier: domain.TierVerified, Status: domain.StatusVerified, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) EvaluateAccess(ctx context.Context, userID string, action domain.Action) (*domain.AccessDecision, error) {
	if m.evaluateAccessFn != nil {
		return m.evaluateAccessFn(ctx, userID, action)
	}
	return &domain.AccessDecision{Action: action, RequiredTier: domain.TierVerified, Satisfied: true}, nil
}

func (m *mockKYCService) StartVerification(ctx context.Context, input domain.StartVerificationInput) (*domain.VerificationCase, error) {
	if m.startVerificationFn != nil {
		return m.startVerificationFn(ctx, input)
	}
	return &domain.VerificationCase{ID: "case-1", UserID: input.UserID, Type: input.Type, Status: domain.StatusPending, RiskScore: 10, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) SubmitDocument(ctx context.Context, input domain.SubmitDocumentInput) error {
	if m.submitDocumentFn != nil {
		return m.submitDocumentFn(ctx, input)
	}
	return nil
}

func (m *mockKYCService) UploadDocument(ctx context.Context, input domain.UploadDocumentInput) (*domain.VerificationCase, error) {
	if m.uploadDocumentFn != nil {
		return m.uploadDocumentFn(ctx, input)
	}
	return &domain.VerificationCase{ID: input.CaseID, UserID: input.UserID, Type: domain.CheckDocument, Status: domain.StatusInReview, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) GetCase(ctx context.Context, userID, caseID string) (*domain.VerificationCase, error) {
	if m.getCaseFn != nil {
		return m.getCaseFn(ctx, userID, caseID)
	}
	return &domain.VerificationCase{ID: caseID, UserID: userID, Type: domain.CheckDocument, Status: domain.StatusInReview, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) ApplyVerdict(ctx context.Context, result domain.VerdictResult) error {
	if m.applyVerdictFn != nil {
		return m.applyVerdictFn(ctx, result)
	}
	return nil
}

func (m *mockKYCService) StartOwnershipClaim(ctx context.Context, input domain.StartOwnershipClaimInput) (*domain.OwnershipClaim, error) {
	if m.startOwnershipClaimFn != nil {
		return m.startOwnershipClaimFn(ctx, input)
	}
	return &domain.OwnershipClaim{ID: "claim-1", UserID: input.UserID, PropertyAddress: input.PropertyAddress, ClaimantName: input.ClaimantName, Status: domain.StatusPending, Method: domain.OwnershipMethodPublicRecord, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) SubmitLicense(ctx context.Context, input domain.SubmitLicenseInput) (*domain.VerificationCase, error) {
	if m.submitLicenseFn != nil {
		return m.submitLicenseFn(ctx, input)
	}
	return &domain.VerificationCase{ID: "case-1", UserID: input.UserID, Type: domain.CheckLicense, Status: domain.StatusVerified, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) StartBusinessVerification(ctx context.Context, input domain.StartBusinessVerificationInput) (*domain.VerificationCase, error) {
	if m.startBusinessVerificationFn != nil {
		return m.startBusinessVerificationFn(ctx, input)
	}
	return &domain.VerificationCase{ID: "case-1", UserID: input.UserID, Type: domain.CheckKYB, Status: domain.StatusVerified, CreatedAt: fixedTime, UpdatedAt: fixedTime}, nil
}

func (m *mockKYCService) DeleteProfile(ctx context.Context, userID string) error {
	if m.deleteProfileFn != nil {
		return m.deleteProfileFn(ctx, userID)
	}
	return nil
}

func errorHandler(c *fiber.Ctx, err error) error {
	var appErr *apperrors.AppError
	switch e := err.(type) {
	case *apperrors.AppError:
		appErr = e
	default:
		appErr = apperrors.Internal("an unexpected error occurred")
	}
	return c.Status(apperrors.HTTPStatus(appErr.Code)).JSON(appErr)
}

func newTestApp(svc domain.KYCService) *fiber.App {
	return newTestAppWithVerifier(svc, &mockWebhookVerifier{})
}

func newTestAppWithVerifier(svc domain.KYCService, v domain.WebhookVerifier) *fiber.App {
	f := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
	})

	f.Use(func(c *fiber.Ctx) error {
		c.Locals("claims", &domain.Claims{UserID: "u1", Email: "test@test.com"})
		return c.Next()
	})

	h := kyc.NewHandler(svc, v)
	f.Post("/kyc/webhooks/provider", h.HandleProviderWebhook)
	f.Get("/kyc/profile", h.GetProfile)
	f.Delete("/kyc/profile", h.DeleteProfile)
	f.Post("/kyc/verifications", h.StartVerification)
	f.Post("/kyc/verifications/:id/documents", h.UploadDocument)
	f.Get("/kyc/verifications/:id", h.GetCase)
	f.Post("/kyc/ownership-claims", h.StartOwnershipClaim)
	f.Post("/kyc/licenses", h.SubmitLicense)
	f.Post("/kyc/business", h.StartBusinessVerification)

	return f
}

func newTestAppNoAuth() *fiber.App {
	f := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
	})
	h := kyc.NewHandler(&mockKYCService{}, &mockWebhookVerifier{})
	f.Get("/kyc/profile", h.GetProfile)
	f.Delete("/kyc/profile", h.DeleteProfile)
	return f
}

func TestHandler_GetProfile(t *testing.T) {
	t.Run("returns profile without action query", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		req, _ := http.NewRequest("GET", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["user_id"] != "u1" {
			t.Errorf("user_id = %v, want u1", body["user_id"])
		}
		if _, ok := body["action_decision"]; ok {
			t.Error("unexpected action_decision field")
		}
	})

	t.Run("returns access decision with action query", func(t *testing.T) {
		svc := &mockKYCService{
			evaluateAccessFn: func(_ context.Context, _ string, _ domain.Action) (*domain.AccessDecision, error) {
				return &domain.AccessDecision{Action: domain.ActionMakeOffer, RequiredTier: domain.TierVerified, Satisfied: true}, nil
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("GET", "/kyc/profile?action=make_offer", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["action_decision"] == nil {
			t.Fatal("expected action_decision field")
		}
		ad := body["action_decision"].(map[string]any)
		if ad["action"] != "make_offer" {
			t.Errorf("decision action = %v, want make_offer", ad["action"])
		}
	})

	t.Run("missing action still succeeds", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		req, _ := http.NewRequest("GET", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("unauthorized when no claims", func(t *testing.T) {
		app := newTestAppNoAuth()
		req, _ := http.NewRequest("GET", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("not found profile returns default T0 response", func(t *testing.T) {
		svc := &mockKYCService{
			getProfileFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, apperrors.NotFound("profile not found")
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("GET", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["tier"] != "T0_ANONYMOUS" {
			t.Errorf("tier = %v, want T0_ANONYMOUS", body["tier"])
		}
		if body["status"] != "pending" {
			t.Errorf("status = %v, want pending", body["status"])
		}
	})

	t.Run("not found profile with action returns T0 plus decision", func(t *testing.T) {
		svc := &mockKYCService{
			getProfileFn: func(_ context.Context, _ string) (*domain.KYCProfile, error) {
				return nil, apperrors.NotFound("profile not found")
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("GET", "/kyc/profile?action=make_offer", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["tier"] != "T0_ANONYMOUS" {
			t.Errorf("tier = %v, want T0_ANONYMOUS", body["tier"])
		}
		if body["action_decision"] == nil {
			t.Fatal("expected action_decision for T0 profile")
		}
	})

	t.Run("service error propagates from evaluate access", func(t *testing.T) {
		svc := &mockKYCService{
			evaluateAccessFn: func(_ context.Context, _ string, _ domain.Action) (*domain.AccessDecision, error) {
				return nil, apperrors.Forbidden("action not allowed")
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("GET", "/kyc/profile?action=make_offer", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})
}

func TestHandler_StartVerification(t *testing.T) {
	t.Run("creates verification case", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"role":"buyer","type":"document"}`
		req, _ := http.NewRequest("POST", "/kyc/verifications", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["id"] != "case-1" {
			t.Errorf("id = %v, want case-1", result["id"])
		}
	})

	t.Run("accepts payout_aml type", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"role":"service_pro","type":"payout_aml"}`
		req, _ := http.NewRequest("POST", "/kyc/verifications", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d, want %d (payout_aml must be an accepted verification type)", resp.StatusCode, http.StatusAccepted)
		}
	})

	t.Run("missing role returns validation error", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"type":"document"}`
		req, _ := http.NewRequest("POST", "/kyc/verifications", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
		}
	})

	t.Run("bad request body returns 400", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		req, _ := http.NewRequest("POST", "/kyc/verifications", strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			startVerificationFn: func(_ context.Context, _ domain.StartVerificationInput) (*domain.VerificationCase, error) {
				return nil, apperrors.Conflict("verification already exists")
			},
		}
		app := newTestApp(svc)
		body := `{"role":"buyer","type":"document"}`
		req, _ := http.NewRequest("POST", "/kyc/verifications", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
		}
	})
}

func multipartDocument(t *testing.T, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="document"; filename="id.jpg"`)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func TestHandler_UploadDocument(t *testing.T) {
	t.Run("uploads document successfully returns 202", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body, contentType := multipartDocument(t, "image/jpeg", []byte("fake-image-bytes"))
		req, _ := http.NewRequest("POST", "/kyc/verifications/case-1/documents", body)
		req.Header.Set("Content-Type", contentType)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["id"] != "case-1" {
			t.Errorf("id = %v, want case-1", result["id"])
		}
	})

	t.Run("missing file returns 400", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		req, _ := http.NewRequest("POST", "/kyc/verifications/case-1/documents", strings.NewReader(""))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=none")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("disallowed content type returns 400", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body, contentType := multipartDocument(t, "application/zip", []byte("zip-bytes"))
		req, _ := http.NewRequest("POST", "/kyc/verifications/case-1/documents", body)
		req.Header.Set("Content-Type", contentType)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			uploadDocumentFn: func(_ context.Context, _ domain.UploadDocumentInput) (*domain.VerificationCase, error) {
				return nil, apperrors.NotFound("case not found")
			},
		}
		app := newTestApp(svc)
		body, contentType := multipartDocument(t, "image/png", []byte("png-bytes"))
		req, _ := http.NewRequest("POST", "/kyc/verifications/case-missing/documents", body)
		req.Header.Set("Content-Type", contentType)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})
}

func TestHandler_GetCase(t *testing.T) {
	t.Run("returns case for owner", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		req, _ := http.NewRequest("GET", "/kyc/verifications/case-1", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["id"] != "case-1" {
			t.Errorf("id = %v, want case-1", result["id"])
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			getCaseFn: func(_ context.Context, _, _ string) (*domain.VerificationCase, error) {
				return nil, apperrors.NotFound("case not found")
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("GET", "/kyc/verifications/case-missing", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})
}

func TestHandler_StartOwnershipClaim(t *testing.T) {
	t.Run("creates ownership claim", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"property_address":"123 Main St","claimant_name":"Alice"}`
		req, _ := http.NewRequest("POST", "/kyc/ownership-claims", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["claimant_name"] != "Alice" {
			t.Errorf("claimant_name = %v, want Alice", result["claimant_name"])
		}
	})

	t.Run("missing property address returns validation error", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"claimant_name":"Alice"}`
		req, _ := http.NewRequest("POST", "/kyc/ownership-claims", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			startOwnershipClaimFn: func(_ context.Context, _ domain.StartOwnershipClaimInput) (*domain.OwnershipClaim, error) {
				return nil, apperrors.Conflict("claim already exists")
			},
		}
		app := newTestApp(svc)
		body := `{"property_address":"123 Main St","claimant_name":"Alice"}`
		req, _ := http.NewRequest("POST", "/kyc/ownership-claims", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
		}
	})
}

func TestHandler_SubmitLicense(t *testing.T) {
	t.Run("submits license successfully", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"role":"agent","license_number":"LIC-123","jurisdiction":"CA"}`
		req, _ := http.NewRequest("POST", "/kyc/licenses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["type"] != "license" {
			t.Errorf("type = %v, want license", result["type"])
		}
	})

	t.Run("invalid role returns validation error", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"role":"buyer","license_number":"LIC-123","jurisdiction":"CA"}`
		req, _ := http.NewRequest("POST", "/kyc/licenses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			submitLicenseFn: func(_ context.Context, _ domain.SubmitLicenseInput) (*domain.VerificationCase, error) {
				return nil, apperrors.Conflict("license already submitted")
			},
		}
		app := newTestApp(svc)
		body := `{"role":"agent","license_number":"LIC-123","jurisdiction":"CA"}`
		req, _ := http.NewRequest("POST", "/kyc/licenses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
		}
	})
}

func TestHandler_StartBusinessVerification(t *testing.T) {
	t.Run("starts business verification", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"business_name":"Acme","registration_number":"EIN-12"}`
		req, _ := http.NewRequest("POST", "/kyc/business", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result["type"] != "kyb" {
			t.Errorf("type = %v, want kyb", result["type"])
		}
	})

	t.Run("missing business name returns validation", func(t *testing.T) {
		app := newTestApp(&mockKYCService{})
		body := `{"registration_number":"EIN-12"}`
		req, _ := http.NewRequest("POST", "/kyc/business", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			startBusinessVerificationFn: func(_ context.Context, _ domain.StartBusinessVerificationInput) (*domain.VerificationCase, error) {
				return nil, apperrors.Conflict("verification already exists")
			},
		}
		app := newTestApp(svc)
		body := `{"business_name":"Acme","registration_number":"EIN-12"}`
		req, _ := http.NewRequest("POST", "/kyc/business", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
		}
	})
}

func TestHandler_DeleteProfile(t *testing.T) {
	t.Run("deletes profile and returns 204", func(t *testing.T) {
		var deleted bool
		svc := &mockKYCService{
			deleteProfileFn: func(_ context.Context, userID string) error {
				deleted = true
				if userID != "u1" {
					t.Errorf("userID = %q, want %q", userID, "u1")
				}
				return nil
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("DELETE", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}
		if !deleted {
			t.Error("expected delete to be called")
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			deleteProfileFn: func(_ context.Context, _ string) error {
				return apperrors.NotFound("profile not found")
			},
		}
		app := newTestApp(svc)
		req, _ := http.NewRequest("DELETE", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("unauthorized when no claims", func(t *testing.T) {
		app := newTestAppNoAuth()
		req, _ := http.NewRequest("DELETE", "/kyc/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
}

func TestHandler_HandleProviderWebhook(t *testing.T) {
	validPayload := `{"event_id":"evt-1","case_id":"case-1","check_type":"document","verdict":"approved","risk_score":10,"raw":{}}`

	t.Run("valid signature and payload returns 200", func(t *testing.T) {
		app := newTestAppWithVerifier(&mockKYCService{}, &mockWebhookVerifier{})
		req, _ := http.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(validPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-KYC-Signature", "abc123")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("bad signature returns 401", func(t *testing.T) {
		v := &mockWebhookVerifier{
			verifyFn: func(_ []byte, _ string) error {
				return domain.ErrInvalidWebhookSignature
			},
		}
		app := newTestAppWithVerifier(&mockKYCService{}, v)
		req, _ := http.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(validPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-KYC-Signature", "bad")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("invalid body returns 400", func(t *testing.T) {
		app := newTestAppWithVerifier(&mockKYCService{}, &mockWebhookVerifier{})
		req, _ := http.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(`not json`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("invalid verdict enum returns 400", func(t *testing.T) {
		app := newTestAppWithVerifier(&mockKYCService{}, &mockWebhookVerifier{})
		body := `{"event_id":"evt-1","case_id":"case-1","check_type":"document","verdict":"unknown","risk_score":10,"raw":{}}`
		req, _ := http.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := &mockKYCService{
			applyVerdictFn: func(_ context.Context, _ domain.VerdictResult) error {
				return apperrors.NotFound("case not found")
			},
		}
		app := newTestAppWithVerifier(svc, &mockWebhookVerifier{})
		req, _ := http.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(validPayload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("duplicate event is idempotent 200", func(t *testing.T) {
		svc := &mockKYCService{
			applyVerdictFn: func(_ context.Context, _ domain.VerdictResult) error {
				return nil
			},
		}
		app := newTestAppWithVerifier(svc, &mockWebhookVerifier{})
		for i := 0; i < 2; i++ {
			req, _ := http.NewRequest("POST", "/kyc/webhooks/provider", strings.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("call %d: status = %d, want %d", i+1, resp.StatusCode, http.StatusOK)
			}
		}
	})
}
