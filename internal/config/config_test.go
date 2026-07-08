package config

import "testing"

func TestResolveKYCReviewMode(t *testing.T) {
	cases := []struct {
		name     string
		explicit string
		urls     []string
		want     string
		wantErr  bool
	}{
		{name: "explicit manual wins over configured providers", explicit: "manual", urls: []string{"https://vendor"}, want: KYCReviewModeManual},
		{name: "explicit provider wins over empty urls", explicit: "provider", urls: []string{"", "", "", ""}, want: KYCReviewModeProvider},
		{name: "unset with all urls empty defaults to manual", explicit: "", urls: []string{"", "", "", ""}, want: KYCReviewModeManual},
		{name: "unset with any url set defaults to provider", explicit: "", urls: []string{"", "https://vendor", "", ""}, want: KYCReviewModeProvider},
		{name: "invalid value is rejected", explicit: "providr", urls: []string{""}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveKYCReviewMode(tc.explicit, tc.urls...)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error for an invalid mode")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}
