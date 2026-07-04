package proxy

import "testing"

func TestCheckAccessTags(t *testing.T) {
	tests := []struct {
		name           string
		userTags       []string
		requiredAccess []string
		wantErr        bool
	}{
		{
			name:           "open model, no user tags",
			userTags:       nil,
			requiredAccess: nil,
			wantErr:        false,
		},
		{
			name:           "open model, user has tags",
			userTags:       []string{"pro"},
			requiredAccess: nil,
			wantErr:        false,
		},
		{
			name:           "required access, user has matching tag",
			userTags:       []string{"pro"},
			requiredAccess: []string{"pro"},
			wantErr:        false,
		},
		{
			name:           "required access, user has one of many",
			userTags:       []string{"preview"},
			requiredAccess: []string{"pro", "preview"},
			wantErr:        false,
		},
		{
			name:           "required access, user has multiple including match",
			userTags:       []string{"standard", "preview"},
			requiredAccess: []string{"preview"},
			wantErr:        false,
		},
		{
			name:           "required access, no user tags",
			userTags:       nil,
			requiredAccess: []string{"pro"},
			wantErr:        true,
		},
		{
			name:           "required access, empty user tags",
			userTags:       []string{},
			requiredAccess: []string{"pro"},
			wantErr:        true,
		},
		{
			name:           "required access, user tags don't match",
			userTags:       []string{"standard"},
			requiredAccess: []string{"pro", "preview"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAccessTags(tt.userTags, tt.requiredAccess)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAccessTags(%v, %v) error = %v, wantErr %v",
					tt.userTags, tt.requiredAccess, err, tt.wantErr)
			}
		})
	}
}

func TestCheckTenantAccess(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		allowedTenants []string
		wantErr        bool
	}{
		{
			name:           "no tenant restriction",
			tenantID:       "",
			allowedTenants: nil,
			wantErr:        false,
		},
		{
			name:           "no restriction, tenant provided",
			tenantID:       "acme",
			allowedTenants: nil,
			wantErr:        false,
		},
		{
			name:           "restriction, matching tenant",
			tenantID:       "acme",
			allowedTenants: []string{"acme", "globex"},
			wantErr:        false,
		},
		{
			name:           "restriction, wrong tenant",
			tenantID:       "initech",
			allowedTenants: []string{"acme", "globex"},
			wantErr:        true,
		},
		{
			name:           "restriction, no tenant provided",
			tenantID:       "",
			allowedTenants: []string{"acme"},
			wantErr:        true,
		},
		{
			name:           "empty allowed list (explicit empty)",
			tenantID:       "acme",
			allowedTenants: []string{},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTenantAccess(tt.tenantID, tt.allowedTenants)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkTenantAccess(%q, %v) error = %v, wantErr %v",
					tt.tenantID, tt.allowedTenants, err, tt.wantErr)
			}
		})
	}
}
