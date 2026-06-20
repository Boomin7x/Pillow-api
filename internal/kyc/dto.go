package kyc

import (
	"github.com/kodiahbertrand/pillow/internal/domain"
)

type StartVerificationRequest struct {
	Role string `json:"role" validate:"required,oneof=buyer renter seller landlord agent lender builder service_pro"`
	Type string `json:"type" validate:"required,oneof=document liveness sanctions payout_aml"`
}

type StartOwnershipClaimRequest struct {
	PropertyAddress string `json:"property_address" validate:"required"`
	ClaimantName    string `json:"claimant_name"    validate:"required"`
}

type SubmitLicenseRequest struct {
	Role          string `json:"role"          validate:"required,oneof=agent lender"`
	LicenseNumber string `json:"license_number" validate:"required"`
	Jurisdiction  string `json:"jurisdiction"   validate:"required"`
}

type StartBusinessVerificationRequest struct {
	BusinessName       string `json:"business_name"        validate:"required"`
	RegistrationNumber string `json:"registration_number"  validate:"required"`
}

type ProfileResponse struct {
	UserID         string                  `json:"user_id"`
	Role           string                  `json:"role"`
	Tier           string                  `json:"tier"`
	Status         string                  `json:"status"`
	Qualifications []string                `json:"qualifications"`
	CreatedAt      string                  `json:"created_at"`
	UpdatedAt      string                  `json:"updated_at"`
	ActionDecision *AccessDecisionResponse `json:"action_decision,omitempty"`
}

type VerificationCaseResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	RiskBand  string `json:"risk_band"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type OwnershipClaimResponse struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	PropertyAddress string `json:"property_address"`
	ClaimantName    string `json:"claimant_name"`
	Status          string `json:"status"`
	Method          string `json:"method"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type AccessDecisionResponse struct {
	Action                 string   `json:"action"`
	RequiredTier           string   `json:"required_tier"`
	RequiredQualifications []string `json:"required_qualifications"`
	MissingQualifications  []string `json:"missing_qualifications"`
	Satisfied              bool     `json:"satisfied"`
}

type ProviderVerdictWebhook struct {
	EventID   string         `json:"event_id"   validate:"required"`
	CaseID    string         `json:"case_id"    validate:"required"`
	CheckType string         `json:"check_type" validate:"required"`
	Verdict   string         `json:"verdict"    validate:"required,oneof=approved rejected review"`
	RiskScore int            `json:"risk_score"`
	Raw       map[string]any `json:"raw,omitempty"`
}

func (w *ProviderVerdictWebhook) ToDomain() domain.VerdictResult {
	return domain.VerdictResult{
		ProviderEventID: w.EventID,
		CaseID:          w.CaseID,
		Type:            domain.CheckType(w.CheckType),
		Verdict:         domain.Verdict(w.Verdict),
		RiskScore:       domain.RiskScore(w.RiskScore),
		RawPayload:      w.Raw,
	}
}

func profileFromDomain(p *domain.KYCProfile) ProfileResponse {
	quals := make([]string, len(p.Qualifications))
	for i, q := range p.Qualifications {
		quals[i] = string(q)
	}
	return ProfileResponse{
		UserID:         p.UserID,
		Role:           string(p.Role),
		Tier:           p.Tier.String(),
		Status:         string(p.Status),
		Qualifications: quals,
		CreatedAt:      p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func caseFromDomain(vc *domain.VerificationCase) VerificationCaseResponse {
	return VerificationCaseResponse{
		ID:        vc.ID,
		UserID:    vc.UserID,
		Type:      string(vc.Type),
		Status:    string(vc.Status),
		RiskBand:  string(vc.RiskScore.Band()),
		CreatedAt: vc.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: vc.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func claimFromDomain(oc *domain.OwnershipClaim) OwnershipClaimResponse {
	return OwnershipClaimResponse{
		ID:              oc.ID,
		UserID:          oc.UserID,
		PropertyAddress: oc.PropertyAddress,
		ClaimantName:    oc.ClaimantName,
		Status:          string(oc.Status),
		Method:          string(oc.Method),
		CreatedAt:       oc.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       oc.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func decisionFromDomain(d *domain.AccessDecision) AccessDecisionResponse {
	reqQuals := make([]string, len(d.RequiredQualifications))
	for i, q := range d.RequiredQualifications {
		reqQuals[i] = string(q)
	}
	missQuals := make([]string, len(d.MissingQualifications))
	for i, q := range d.MissingQualifications {
		missQuals[i] = string(q)
	}
	return AccessDecisionResponse{
		Action:                 string(d.Action),
		RequiredTier:           d.RequiredTier.String(),
		RequiredQualifications: reqQuals,
		MissingQualifications:  missQuals,
		Satisfied:              d.Satisfied,
	}
}
