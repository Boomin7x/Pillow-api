package domain

import (
	"context"
	"errors"
	"time"
)

type Tier int

const (
	TierAnonymous Tier = iota
	TierIdentified
	TierVerified
	TierRegulated
)

func (t Tier) String() string {
	switch t {
	case TierAnonymous:
		return "T0_ANONYMOUS"
	case TierIdentified:
		return "T1_IDENTIFIED"
	case TierVerified:
		return "T2_VERIFIED"
	case TierRegulated:
		return "T3_REGULATED"
	default:
		return "UNKNOWN"
	}
}

func (t Tier) MeetsOrExceeds(required Tier) bool {
	return t >= required
}

type Role string

const (
	RoleBuyer      Role = "buyer"
	RoleRenter     Role = "renter"
	RoleSeller     Role = "seller"
	RoleLandlord   Role = "landlord"
	RoleAgent      Role = "agent"
	RoleLender     Role = "lender"
	RoleBuilder    Role = "builder"
	RoleServicePro Role = "service_pro"
)

func (r Role) Valid() bool {
	switch r {
	case RoleBuyer, RoleRenter, RoleSeller, RoleLandlord,
		RoleAgent, RoleLender, RoleBuilder, RoleServicePro:
		return true
	default:
		return false
	}
}

type VerificationStatus string

const (
	StatusPending  VerificationStatus = "pending"
	StatusInReview VerificationStatus = "in_review"
	StatusVerified VerificationStatus = "verified"
	StatusRejected VerificationStatus = "rejected"
	StatusExpired  VerificationStatus = "expired"
)

func (s VerificationStatus) IsTerminal() bool {
	switch s {
	case StatusVerified, StatusRejected, StatusExpired:
		return true
	default:
		return false
	}
}

type CheckType string

const (
	CheckDocument  CheckType = "document"
	CheckLiveness  CheckType = "liveness"
	CheckSanctions CheckType = "sanctions"
	CheckOwnership CheckType = "ownership"
	CheckLicense   CheckType = "license"
	CheckKYB       CheckType = "kyb"
	CheckPayoutAML CheckType = "payout_aml"
)

type Qualification string

const (
	QualificationOwnership Qualification = "ownership"
	QualificationPayoutAML Qualification = "payout_aml"
	QualificationLicense   Qualification = "license"
	QualificationKYB       Qualification = "kyb"
)

type Verdict string

const (
	VerdictApproved Verdict = "approved"
	VerdictRejected Verdict = "rejected"
	VerdictReview   Verdict = "review"
)

type RiskBand string

const (
	RiskBandLow    RiskBand = "low"
	RiskBandMedium RiskBand = "medium"
	RiskBandHigh   RiskBand = "high"
)

type RiskScore int

const (
	riskMediumThreshold RiskScore = 34
	riskHighThreshold   RiskScore = 67
)

func (r RiskScore) Band() RiskBand {
	switch {
	case r >= riskHighThreshold:
		return RiskBandHigh
	case r >= riskMediumThreshold:
		return RiskBandMedium
	default:
		return RiskBandLow
	}
}

func (r RiskScore) RequiresStepUp() bool {
	return r.Band() != RiskBandLow
}

func (r RiskScore) RequiresManualReview() bool {
	return r.Band() == RiskBandHigh
}

type Action string

const (
	ActionBrowse            Action = "browse"
	ActionSaveSearch        Action = "save_search"
	ActionContactAgent      Action = "contact_agent"
	ActionRequestTour       Action = "request_tour"
	ActionMakeOffer         Action = "make_offer"
	ActionSubmitApplication Action = "submit_application"
	ActionListProperty      Action = "list_property"
	ActionReceivePayout     Action = "receive_payout"
	ActionReceiveRent       Action = "receive_rent"
	ActionOperateAsAgent    Action = "operate_as_agent"
	ActionOperateAsLender   Action = "operate_as_lender"
	ActionOperateAsBuilder  Action = "operate_as_builder"
)

type AccessRequirement struct {
	MinTier        Tier
	Qualifications []Qualification
}

func RequirementFor(action Action) (AccessRequirement, bool) {
	switch action {
	case ActionBrowse:
		return AccessRequirement{MinTier: TierAnonymous}, true
	case ActionSaveSearch, ActionContactAgent, ActionRequestTour:
		return AccessRequirement{MinTier: TierIdentified}, true
	case ActionMakeOffer, ActionSubmitApplication:
		return AccessRequirement{MinTier: TierVerified}, true
	case ActionListProperty:
		return AccessRequirement{
			MinTier:        TierVerified,
			Qualifications: []Qualification{QualificationOwnership},
		}, true
	case ActionReceivePayout:
		return AccessRequirement{
			MinTier:        TierVerified,
			Qualifications: []Qualification{QualificationPayoutAML},
		}, true
	case ActionReceiveRent:
		return AccessRequirement{
			MinTier:        TierVerified,
			Qualifications: []Qualification{QualificationOwnership, QualificationPayoutAML},
		}, true
	case ActionOperateAsAgent, ActionOperateAsLender:
		return AccessRequirement{
			MinTier:        TierRegulated,
			Qualifications: []Qualification{QualificationLicense},
		}, true
	case ActionOperateAsBuilder:
		return AccessRequirement{
			MinTier:        TierRegulated,
			Qualifications: []Qualification{QualificationKYB},
		}, true
	default:
		return AccessRequirement{}, false
	}
}

type KYCProfile struct {
	UserID         string
	Role           Role
	Tier           Tier
	Status         VerificationStatus
	Qualifications []Qualification
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (p *KYCProfile) HasQualification(q Qualification) bool {
	for _, owned := range p.Qualifications {
		if owned == q {
			return true
		}
	}
	return false
}

func (p *KYCProfile) GrantQualification(q Qualification) {
	if p.HasQualification(q) {
		return
	}
	p.Qualifications = append(p.Qualifications, q)
}

func (p *KYCProfile) RequiredTierFor(action Action) (Tier, bool) {
	requirement, known := RequirementFor(action)
	if !known {
		return TierAnonymous, false
	}
	return requirement.MinTier, true
}

func (p *KYCProfile) MissingQualificationsFor(action Action) []Qualification {
	requirement, known := RequirementFor(action)
	if !known {
		return nil
	}
	var missing []Qualification
	for _, required := range requirement.Qualifications {
		if !p.HasQualification(required) {
			missing = append(missing, required)
		}
	}
	return missing
}

func (p *KYCProfile) CanPerform(action Action) bool {
	requirement, known := RequirementFor(action)
	if !known {
		return false
	}
	if !p.Tier.MeetsOrExceeds(requirement.MinTier) {
		return false
	}
	for _, required := range requirement.Qualifications {
		if !p.HasQualification(required) {
			return false
		}
	}
	return true
}

type TransitionEvent string

const (
	EventSubmit  TransitionEvent = "submit"
	EventApprove TransitionEvent = "approve"
	EventReject  TransitionEvent = "reject"
	EventExpire  TransitionEvent = "expire"
	EventRetry   TransitionEvent = "retry"
)

var ErrIllegalTransition = errors.New("kyc: illegal verification status transition")
var ErrInvalidWebhookSignature = errors.New("kyc: invalid webhook signature")

func NextStatus(current VerificationStatus, event TransitionEvent) (VerificationStatus, error) {
	switch {
	case current == StatusPending && event == EventSubmit:
		return StatusInReview, nil
	case current == StatusInReview && event == EventApprove:
		return StatusVerified, nil
	case current == StatusInReview && event == EventReject:
		return StatusRejected, nil
	case current == StatusVerified && event == EventExpire:
		return StatusExpired, nil
	case current == StatusRejected && event == EventRetry:
		return StatusPending, nil
	case current == StatusExpired && event == EventRetry:
		return StatusPending, nil
	default:
		return current, ErrIllegalTransition
	}
}

type VerificationCase struct {
	ID        string
	UserID    string
	Type      CheckType
	Status    VerificationStatus
	RiskScore RiskScore
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Check struct {
	ID              string
	CaseID          string
	Type            CheckType
	Status          VerificationStatus
	Verdict         Verdict
	ProviderEventID string
	RiskScore       RiskScore
	RawPayload      map[string]any
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (c *Check) IsTerminal() bool {
	return c.Status.IsTerminal()
}

type OwnershipVerificationMethod string

const (
	OwnershipMethodPublicRecord OwnershipVerificationMethod = "public_record"
	OwnershipMethodDocument     OwnershipVerificationMethod = "document"
	OwnershipMethodPostcard     OwnershipVerificationMethod = "postcard"
	OwnershipMethodManualReview OwnershipVerificationMethod = "manual_review"
)

type OwnershipClaim struct {
	ID              string
	UserID          string
	PropertyAddress string
	ClaimantName    string
	Status          VerificationStatus
	Method          OwnershipVerificationMethod
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AuditEvent struct {
	ID        string
	UserID    string
	EventType string
	Tier      Tier
	Metadata  map[string]any
	CreatedAt time.Time
}

type RiskSignals struct {
	DeviceFingerprint string
	IPAddress         string
	GeoCountry        string
	Velocity          int
	DocumentTampered  bool
}

type AccessDecision struct {
	Action                 Action
	RequiredTier           Tier
	RequiredQualifications []Qualification
	MissingQualifications  []Qualification
	Satisfied              bool
}

type DocumentReference string

type DocumentUpload struct {
	UserID      string
	ContentType string
	Content     []byte
}

type ProviderCheckResult struct {
	ProviderEventID string
	Verdict         Verdict
	RiskScore       RiskScore
	RawPayload      map[string]any
}

type DocumentVerificationRequest struct {
	UserID            string
	DocumentReference DocumentReference
}

type LivenessRequest struct {
	UserID            string
	DocumentReference DocumentReference
}

type SanctionsRequest struct {
	UserID   string
	FullName string
}

type OwnershipVerificationRequest struct {
	UserID          string
	PropertyAddress string
	ClaimantName    string
}

type LicenseVerificationRequest struct {
	UserID        string
	LicenseNumber string
	Jurisdiction  string
}

type BusinessVerificationRequest struct {
	UserID             string
	BusinessName       string
	RegistrationNumber string
}

type StartVerificationInput struct {
	UserID  string
	Role    Role
	Type    CheckType
	Signals RiskSignals
}

type SubmitDocumentInput struct {
	UserID            string
	CaseID            string
	DocumentReference DocumentReference
}

type UploadDocumentInput struct {
	UserID      string
	CaseID      string
	ContentType string
	Content     []byte
}

type VerdictResult struct {
	ProviderEventID string
	CaseID          string
	Type            CheckType
	Verdict         Verdict
	RiskScore       RiskScore
	RawPayload      map[string]any
}

type StartOwnershipClaimInput struct {
	UserID          string
	PropertyAddress string
	ClaimantName    string
}

type SubmitLicenseInput struct {
	UserID        string
	Role          Role
	LicenseNumber string
	Jurisdiction  string
}

type StartBusinessVerificationInput struct {
	UserID             string
	Role               Role
	BusinessName       string
	RegistrationNumber string
}

type KYCRepository interface {
	CreateProfile(ctx context.Context, profile *KYCProfile) error
	FindProfileByUserID(ctx context.Context, userID string) (*KYCProfile, error)
	UpdateProfile(ctx context.Context, profile *KYCProfile) error
	TombstoneProfile(ctx context.Context, userID string) error

	CreateCase(ctx context.Context, verificationCase *VerificationCase) error
	FindCaseByID(ctx context.Context, id string) (*VerificationCase, error)
	UpdateCase(ctx context.Context, verificationCase *VerificationCase) error
	ListPendingCases(ctx context.Context, limit int) ([]VerificationCase, error)
	ListCasesStuckInReview(ctx context.Context, olderThan time.Time, limit int) ([]VerificationCase, error)

	CreateCheck(ctx context.Context, check *Check) error
	FindCheckByProviderEventID(ctx context.Context, providerEventID string) (*Check, error)
	UpdateCheck(ctx context.Context, check *Check) error
	ListChecksExpiringBefore(ctx context.Context, t time.Time, limit int) ([]Check, error)

	CreateOwnershipClaim(ctx context.Context, claim *OwnershipClaim) error
	FindOwnershipClaimByID(ctx context.Context, id string) (*OwnershipClaim, error)
	UpdateOwnershipClaim(ctx context.Context, claim *OwnershipClaim) error

	ListProfilesForRescreen(ctx context.Context, minTier Tier, limit int, cursor string) ([]KYCProfile, error)
}

type AuditRepository interface {
	Append(ctx context.Context, event *AuditEvent) error
	ListByUserID(ctx context.Context, userID string) ([]AuditEvent, error)
}

type KYCService interface {
	GetProfile(ctx context.Context, userID string) (*KYCProfile, error)
	EvaluateAccess(ctx context.Context, userID string, action Action) (*AccessDecision, error)
	StartVerification(ctx context.Context, input StartVerificationInput) (*VerificationCase, error)
	SubmitDocument(ctx context.Context, input SubmitDocumentInput) error
	UploadDocument(ctx context.Context, input UploadDocumentInput) (*VerificationCase, error)
	GetCase(ctx context.Context, userID, caseID string) (*VerificationCase, error)
	ApplyVerdict(ctx context.Context, result VerdictResult) error
	StartOwnershipClaim(ctx context.Context, input StartOwnershipClaimInput) (*OwnershipClaim, error)
	SubmitLicense(ctx context.Context, input SubmitLicenseInput) (*VerificationCase, error)
	StartBusinessVerification(ctx context.Context, input StartBusinessVerificationInput) (*VerificationCase, error)
	DeleteProfile(ctx context.Context, userID string) error
}

type IdentityVerifier interface {
	VerifyDocument(ctx context.Context, request DocumentVerificationRequest) (*ProviderCheckResult, error)
	VerifyLiveness(ctx context.Context, request LivenessRequest) (*ProviderCheckResult, error)
}

type SanctionsScreener interface {
	Screen(ctx context.Context, request SanctionsRequest) (*ProviderCheckResult, error)
}

type OwnershipVerifier interface {
	VerifyOwnership(ctx context.Context, request OwnershipVerificationRequest) (*ProviderCheckResult, error)
}

type LicenseVerifier interface {
	VerifyLicense(ctx context.Context, request LicenseVerificationRequest) (*ProviderCheckResult, error)
}

type BusinessVerifier interface {
	VerifyBusiness(ctx context.Context, request BusinessVerificationRequest) (*ProviderCheckResult, error)
}

type DocumentVault interface {
	Store(ctx context.Context, upload DocumentUpload) (DocumentReference, error)
	SignedURL(ctx context.Context, reference DocumentReference, ttl time.Duration) (string, error)
	Purge(ctx context.Context, reference DocumentReference) error
	PurgeExpired(ctx context.Context) (int, error)
	PurgeUser(ctx context.Context, userID string) (int, error)
}

type TierCache interface {
	Get(ctx context.Context, userID string) (*KYCProfile, error)
	Set(ctx context.Context, userID string, profile *KYCProfile, ttl time.Duration) error
	Del(ctx context.Context, userID string) error
}

type WebhookVerifier interface {
	Verify(payload []byte, signature string) error
}

type Notification struct {
	UserID    string
	EventType string
	CaseID    string
	Status    VerificationStatus
}

type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

type VerificationJob struct {
	CaseID             string
	ClaimID            string
	UserID             string
	Type               CheckType
	LicenseNumber      string
	Jurisdiction       string
	BusinessName       string
	RegistrationNumber string
	PropertyAddress    string
	ClaimantName       string
	DocumentReference  DocumentReference
}

type VerificationQueue interface {
	Enqueue(ctx context.Context, job VerificationJob) error
	Dequeue(ctx context.Context, timeout time.Duration) (*VerificationJob, error)
	Len(ctx context.Context) (int, error)
}
