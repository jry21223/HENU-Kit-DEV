package career

import "testing"

func TestValidProfileInputRequiresTargetRoles(t *testing.T) {
	if validProfileInput(profileInput{}) {
		t.Fatal("empty target roles were accepted")
	}
	if validProfileInput(profileInput{TargetRoles: "   "}) {
		t.Fatal("blank target roles were accepted")
	}
	if !validProfileInput(profileInput{TargetRoles: "后端开发"}) {
		t.Fatal("a profile with target roles was rejected")
	}
}
