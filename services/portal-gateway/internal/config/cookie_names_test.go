package config

import "testing"

func TestValidateLocalCookieNames(t *testing.T) {
	tests := []struct {
		name    string
		oauth   string
		session string
		wantErr bool
	}{
		{name: "valid distinct names", oauth: "henukit_portal_oauth_local", session: "henukit_portal_session_local"},
		{name: "invalid oauth name", oauth: "oauth;bad", session: "session_local", wantErr: true},
		{name: "invalid session name", oauth: "oauth_local", session: "session bad", wantErr: true},
		{name: "host prefix is production only", oauth: "__Host-oauth", session: "session_local", wantErr: true},
		{name: "names must be distinct", oauth: "same_name", session: "same_name", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLocalCookieNames(test.oauth, test.session)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateLocalCookieNames() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
