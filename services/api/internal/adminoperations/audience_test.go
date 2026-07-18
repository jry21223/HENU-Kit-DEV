package adminoperations

import (
	"testing"

	"final-review-platform/services/api/internal/platform/model"
)

func TestNoticeAudienceMatchesAllSchoolAndMajor(t *testing.T) {
	school := "11111111-1111-4111-8111-111111111111"
	major := "22222222-2222-4222-8222-222222222222"
	user := model.User{SchoolID: &school, MajorID: &major}
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"empty defaults to all", "", true},
		{"all verified", `["all_verified_users"]`, true},
		{"matching school", `["school:` + school + `"]`, true},
		{"matching major", `["major:` + major + `"]`, true},
		{"different school", `["school:33333333-3333-4333-8333-333333333333"]`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noticeAudienceMatches([]byte(tt.raw), user); got != tt.want {
				t.Fatalf("noticeAudienceMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
