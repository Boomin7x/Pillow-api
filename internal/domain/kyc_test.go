package domain_test

import (
	"errors"
	"testing"

	"github.com/kodiahbertrand/pillow/internal/domain"
)

func TestTierString(t *testing.T) {
	tests := []struct {
		name string
		tier domain.Tier
		want string
	}{
		{"anonymous renders T0", domain.TierAnonymous, "T0_ANONYMOUS"},
		{"identified renders T1", domain.TierIdentified, "T1_IDENTIFIED"},
		{"verified renders T2", domain.TierVerified, "T2_VERIFIED"},
		{"regulated renders T3", domain.TierRegulated, "T3_REGULATED"},
		{"out of range renders unknown", domain.Tier(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tier.String(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTierMeetsOrExceeds(t *testing.T) {
	tests := []struct {
		name     string
		have     domain.Tier
		required domain.Tier
		want     bool
	}{
		{"equal tiers satisfy", domain.TierVerified, domain.TierVerified, true},
		{"higher tier satisfies", domain.TierRegulated, domain.TierVerified, true},
		{"lower tier does not satisfy", domain.TierIdentified, domain.TierVerified, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.have.MeetsOrExceeds(tt.required); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoleValid(t *testing.T) {
	tests := []struct {
		name string
		role domain.Role
		want bool
	}{
		{"buyer is valid", domain.RoleBuyer, true},
		{"renter is valid", domain.RoleRenter, true},
		{"seller is valid", domain.RoleSeller, true},
		{"landlord is valid", domain.RoleLandlord, true},
		{"agent is valid", domain.RoleAgent, true},
		{"lender is valid", domain.RoleLender, true},
		{"builder is valid", domain.RoleBuilder, true},
		{"service pro is valid", domain.RoleServicePro, true},
		{"empty role is invalid", domain.Role(""), false},
		{"unknown role is invalid", domain.Role("squatter"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.Valid(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerificationStatusIsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status domain.VerificationStatus
		want   bool
	}{
		{"pending is not terminal", domain.StatusPending, false},
		{"in review is not terminal", domain.StatusInReview, false},
		{"verified is terminal", domain.StatusVerified, true},
		{"rejected is terminal", domain.StatusRejected, true},
		{"expired is terminal", domain.StatusExpired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRiskScoreBand(t *testing.T) {
	tests := []struct {
		name  string
		score domain.RiskScore
		want  domain.RiskBand
	}{
		{"zero is low", domain.RiskScore(0), domain.RiskBandLow},
		{"just below medium is low", domain.RiskScore(33), domain.RiskBandLow},
		{"medium threshold is medium", domain.RiskScore(34), domain.RiskBandMedium},
		{"just below high is medium", domain.RiskScore(66), domain.RiskBandMedium},
		{"high threshold is high", domain.RiskScore(67), domain.RiskBandHigh},
		{"max is high", domain.RiskScore(100), domain.RiskBandHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.score.Band(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRiskScoreRequiresStepUp(t *testing.T) {
	tests := []struct {
		name  string
		score domain.RiskScore
		want  bool
	}{
		{"low does not require step up", domain.RiskScore(10), false},
		{"medium requires step up", domain.RiskScore(40), true},
		{"high requires step up", domain.RiskScore(80), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.score.RequiresStepUp(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRiskScoreRequiresManualReview(t *testing.T) {
	tests := []struct {
		name  string
		score domain.RiskScore
		want  bool
	}{
		{"low does not require review", domain.RiskScore(10), false},
		{"medium does not require review", domain.RiskScore(40), false},
		{"high requires review", domain.RiskScore(80), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.score.RequiresManualReview(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequirementFor(t *testing.T) {
	tests := []struct {
		name           string
		action         domain.Action
		wantKnown      bool
		wantMinTier    domain.Tier
		wantQualifiers []domain.Qualification
	}{
		{"browse needs anonymous", domain.ActionBrowse, true, domain.TierAnonymous, nil},
		{"save search needs identified", domain.ActionSaveSearch, true, domain.TierIdentified, nil},
		{"contact agent needs identified", domain.ActionContactAgent, true, domain.TierIdentified, nil},
		{"request tour needs identified", domain.ActionRequestTour, true, domain.TierIdentified, nil},
		{"make offer needs verified", domain.ActionMakeOffer, true, domain.TierVerified, nil},
		{"submit application needs verified", domain.ActionSubmitApplication, true, domain.TierVerified, nil},
		{"list property needs ownership", domain.ActionListProperty, true, domain.TierVerified, []domain.Qualification{domain.QualificationOwnership}},
		{"receive payout needs aml", domain.ActionReceivePayout, true, domain.TierVerified, []domain.Qualification{domain.QualificationPayoutAML}},
		{"receive rent needs ownership and aml", domain.ActionReceiveRent, true, domain.TierVerified, []domain.Qualification{domain.QualificationOwnership, domain.QualificationPayoutAML}},
		{"operate as agent needs license", domain.ActionOperateAsAgent, true, domain.TierRegulated, []domain.Qualification{domain.QualificationLicense}},
		{"operate as lender needs license", domain.ActionOperateAsLender, true, domain.TierRegulated, []domain.Qualification{domain.QualificationLicense}},
		{"operate as builder needs kyb", domain.ActionOperateAsBuilder, true, domain.TierRegulated, []domain.Qualification{domain.QualificationKYB}},
		{"unknown action is not known", domain.Action("teleport"), false, domain.TierAnonymous, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requirement, known := domain.RequirementFor(tt.action)
			if known != tt.wantKnown {
				t.Fatalf("known: got %v, want %v", known, tt.wantKnown)
			}
			if requirement.MinTier != tt.wantMinTier {
				t.Errorf("min tier: got %v, want %v", requirement.MinTier, tt.wantMinTier)
			}
			if !qualificationsEqual(requirement.Qualifications, tt.wantQualifiers) {
				t.Errorf("qualifications: got %v, want %v", requirement.Qualifications, tt.wantQualifiers)
			}
		})
	}
}

func TestKYCProfileHasQualification(t *testing.T) {
	profile := &domain.KYCProfile{Qualifications: []domain.Qualification{domain.QualificationOwnership}}
	if !profile.HasQualification(domain.QualificationOwnership) {
		t.Errorf("got false, want true for owned qualification")
	}
	if profile.HasQualification(domain.QualificationLicense) {
		t.Errorf("got true, want false for absent qualification")
	}
}

func TestKYCProfileGrantQualification(t *testing.T) {
	profile := &domain.KYCProfile{}

	profile.GrantQualification(domain.QualificationOwnership)
	if len(profile.Qualifications) != 1 {
		t.Fatalf("after first grant: got %d qualifications, want 1", len(profile.Qualifications))
	}

	profile.GrantQualification(domain.QualificationOwnership)
	if len(profile.Qualifications) != 1 {
		t.Errorf("after duplicate grant: got %d qualifications, want 1", len(profile.Qualifications))
	}

	profile.GrantQualification(domain.QualificationPayoutAML)
	if len(profile.Qualifications) != 2 {
		t.Errorf("after second distinct grant: got %d qualifications, want 2", len(profile.Qualifications))
	}
}

func TestKYCProfileRequiredTierFor(t *testing.T) {
	profile := &domain.KYCProfile{}

	tier, known := profile.RequiredTierFor(domain.ActionMakeOffer)
	if !known || tier != domain.TierVerified {
		t.Errorf("known action: got (%v, %v), want (T2_VERIFIED, true)", tier, known)
	}

	tier, known = profile.RequiredTierFor(domain.Action("teleport"))
	if known || tier != domain.TierAnonymous {
		t.Errorf("unknown action: got (%v, %v), want (T0_ANONYMOUS, false)", tier, known)
	}
}

func TestKYCProfileMissingQualificationsFor(t *testing.T) {
	tests := []struct {
		name    string
		profile *domain.KYCProfile
		action  domain.Action
		want    []domain.Qualification
	}{
		{
			"all qualifications present returns none",
			&domain.KYCProfile{Qualifications: []domain.Qualification{domain.QualificationOwnership, domain.QualificationPayoutAML}},
			domain.ActionReceiveRent,
			nil,
		},
		{
			"missing one qualification is reported",
			&domain.KYCProfile{Qualifications: []domain.Qualification{domain.QualificationOwnership}},
			domain.ActionReceiveRent,
			[]domain.Qualification{domain.QualificationPayoutAML},
		},
		{
			"action without qualifications returns none",
			&domain.KYCProfile{},
			domain.ActionMakeOffer,
			nil,
		},
		{
			"unknown action returns none",
			&domain.KYCProfile{},
			domain.Action("teleport"),
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.MissingQualificationsFor(tt.action); !qualificationsEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKYCProfileCanPerform(t *testing.T) {
	tests := []struct {
		name    string
		profile *domain.KYCProfile
		action  domain.Action
		want    bool
	}{
		{
			"unknown action is denied",
			&domain.KYCProfile{Tier: domain.TierRegulated},
			domain.Action("teleport"),
			false,
		},
		{
			"insufficient tier is denied",
			&domain.KYCProfile{Tier: domain.TierIdentified},
			domain.ActionMakeOffer,
			false,
		},
		{
			"sufficient tier without qualification is denied",
			&domain.KYCProfile{Tier: domain.TierVerified},
			domain.ActionListProperty,
			false,
		},
		{
			"sufficient tier with qualification is allowed",
			&domain.KYCProfile{Tier: domain.TierVerified, Qualifications: []domain.Qualification{domain.QualificationOwnership}},
			domain.ActionListProperty,
			true,
		},
		{
			"tier only action is allowed at tier",
			&domain.KYCProfile{Tier: domain.TierVerified},
			domain.ActionMakeOffer,
			true,
		},
		{
			"anonymous can browse",
			&domain.KYCProfile{Tier: domain.TierAnonymous},
			domain.ActionBrowse,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.CanPerform(tt.action); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextStatus(t *testing.T) {
	tests := []struct {
		name    string
		current domain.VerificationStatus
		event   domain.TransitionEvent
		want    domain.VerificationStatus
		wantErr bool
	}{
		{"pending submits to in review", domain.StatusPending, domain.EventSubmit, domain.StatusInReview, false},
		{"in review approves to verified", domain.StatusInReview, domain.EventApprove, domain.StatusVerified, false},
		{"in review rejects to rejected", domain.StatusInReview, domain.EventReject, domain.StatusRejected, false},
		{"verified expires to expired", domain.StatusVerified, domain.EventExpire, domain.StatusExpired, false},
		{"rejected retries to pending", domain.StatusRejected, domain.EventRetry, domain.StatusPending, false},
		{"expired retries to pending", domain.StatusExpired, domain.EventRetry, domain.StatusPending, false},
		{"approve on pending is illegal", domain.StatusPending, domain.EventApprove, domain.StatusPending, true},
		{"submit on verified is illegal", domain.StatusVerified, domain.EventSubmit, domain.StatusVerified, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NextStatus(tt.current, tt.event)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrIllegalTransition) {
					t.Fatalf("got err %v, want ErrIllegalTransition", err)
				}
			} else if err != nil {
				t.Fatalf("got unexpected err %v", err)
			}
			if got != tt.want {
				t.Errorf("status: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckIsTerminal(t *testing.T) {
	pending := &domain.Check{Status: domain.StatusPending}
	if pending.IsTerminal() {
		t.Errorf("pending check: got true, want false")
	}
	verified := &domain.Check{Status: domain.StatusVerified}
	if !verified.IsTerminal() {
		t.Errorf("verified check: got false, want true")
	}
}

func qualificationsEqual(a, b []domain.Qualification) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
