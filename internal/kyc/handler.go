package kyc

import (
	"errors"
	"io"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/internal/apperrors"
	"github.com/kodiahbertrand/pillow/internal/domain"
	"github.com/kodiahbertrand/pillow/internal/middleware"
)

const maxDocumentBytes = 10 << 20

var allowedDocumentContentTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"application/pdf": true,
}

type kycHandler struct {
	service   domain.KYCService
	validator *validator.Validate
	verifier  domain.WebhookVerifier
}

func NewHandler(svc domain.KYCService, verifier domain.WebhookVerifier) *kycHandler {
	return &kycHandler{
		service:   svc,
		validator: validator.New(),
		verifier:  verifier,
	}
}

func (h *kycHandler) HandleProviderWebhook(c *fiber.Ctx) error {
	body := c.Body()
	if err := h.verifier.Verify(body, c.Get("X-KYC-Signature")); err != nil {
		return apperrors.Unauthorized("invalid webhook signature")
	}

	var dto ProviderVerdictWebhook
	if err := c.BodyParser(&dto); err != nil {
		return apperrors.BadRequest("invalid webhook payload")
	}
	if err := h.validator.Struct(dto); err != nil {
		return apperrors.BadRequest("invalid webhook payload")
	}

	if err := h.service.ApplyVerdict(c.UserContext(), dto.ToDomain()); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *kycHandler) GetProfile(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	profile, err := h.service.GetProfile(c.UserContext(), claims.UserID)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound {
			resp := ProfileResponse{
				UserID: claims.UserID, Tier: domain.TierAnonymous.String(),
				Status: string(domain.StatusPending), Qualifications: []string{},
			}
			if actionStr := c.Query("action"); actionStr != "" {
				action := domain.Action(actionStr)
				decision, evalErr := h.service.EvaluateAccess(c.UserContext(), claims.UserID, action)
				if evalErr == nil {
					d := decisionFromDomain(decision)
					resp.ActionDecision = &d
				}
			}
			return c.JSON(resp)
		}
		return err
	}

	resp := profileFromDomain(profile)

	if actionStr := c.Query("action"); actionStr != "" {
		action := domain.Action(actionStr)
		decision, err := h.service.EvaluateAccess(c.UserContext(), claims.UserID, action)
		if err != nil {
			return err
		}
		d := decisionFromDomain(decision)
		resp.ActionDecision = &d
	}

	return c.JSON(resp)
}

func (h *kycHandler) StartVerification(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	var req StartVerificationRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	vc, err := h.service.StartVerification(c.UserContext(), domain.StartVerificationInput{
		UserID:  claims.UserID,
		Role:    domain.Role(req.Role),
		Type:    domain.CheckType(req.Type),
		Signals: signalsFromRequest(c),
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(caseFromDomain(vc))
}

func signalsFromRequest(c *fiber.Ctx) domain.RiskSignals {
	return domain.RiskSignals{
		DeviceFingerprint: c.Get("X-Device-Fingerprint"),
		IPAddress:         c.IP(),
		GeoCountry:        c.Get("CF-IPCountry"),
	}
}

func (h *kycHandler) UploadDocument(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	caseID := c.Params("id")
	if caseID == "" {
		return apperrors.BadRequest("missing case id")
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return apperrors.BadRequest("document file is required")
	}
	if fileHeader.Size > maxDocumentBytes {
		return apperrors.BadRequest("document exceeds maximum size")
	}
	contentType := fileHeader.Header.Get("Content-Type")
	if !allowedDocumentContentTypes[contentType] {
		return apperrors.BadRequest("unsupported document content type")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return apperrors.BadRequest("could not read document file")
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes))
	if err != nil {
		return apperrors.BadRequest("could not read document file")
	}

	vc, err := h.service.UploadDocument(c.UserContext(), domain.UploadDocumentInput{
		UserID:      claims.UserID,
		CaseID:      caseID,
		ContentType: contentType,
		Content:     content,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(caseFromDomain(vc))
}

func (h *kycHandler) GetCase(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	caseID := c.Params("id")
	if caseID == "" {
		return apperrors.BadRequest("missing case id")
	}

	vc, err := h.service.GetCase(c.UserContext(), claims.UserID, caseID)
	if err != nil {
		return err
	}

	return c.JSON(caseFromDomain(vc))
}

func (h *kycHandler) StartOwnershipClaim(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	var req StartOwnershipClaimRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	oc, err := h.service.StartOwnershipClaim(c.UserContext(), domain.StartOwnershipClaimInput{
		UserID:          claims.UserID,
		PropertyAddress: req.PropertyAddress,
		ClaimantName:    req.ClaimantName,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(claimFromDomain(oc))
}

func (h *kycHandler) SubmitLicense(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	var req SubmitLicenseRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	vc, err := h.service.SubmitLicense(c.UserContext(), domain.SubmitLicenseInput{
		UserID:        claims.UserID,
		Role:          domain.Role(req.Role),
		LicenseNumber: req.LicenseNumber,
		Jurisdiction:  req.Jurisdiction,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(caseFromDomain(vc))
}

func (h *kycHandler) DeleteProfile(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}
	if err := h.service.DeleteProfile(c.UserContext(), claims.UserID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *kycHandler) StartBusinessVerification(c *fiber.Ctx) error {
	claims := middleware.Claims(c)
	if claims == nil {
		return apperrors.Unauthorized("not authenticated")
	}

	var req StartBusinessVerificationRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.BadRequest("invalid request body")
	}
	if err := h.validator.Struct(req); err != nil {
		return apperrors.Validation(err.Error())
	}

	vc, err := h.service.StartBusinessVerification(c.UserContext(), domain.StartBusinessVerificationInput{
		UserID:             claims.UserID,
		Role:               domain.RoleBuilder,
		BusinessName:       req.BusinessName,
		RegistrationNumber: req.RegistrationNumber,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(caseFromDomain(vc))
}
