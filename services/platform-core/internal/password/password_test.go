package password

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateUsesUnicodeCodePointsWithoutChangingPassword(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		wantErr  bool
	}{
		{name: "ten Unicode code points", email: "student@henu.edu.cn", password: "河大密码短语甲乙丙丁", wantErr: false},
		{name: "nine Unicode code points", email: "student@henu.edu.cn", password: "河大密码甲乙丙丁戊", wantErr: true},
		{name: "exact normalized email local part", email: "student@henu.edu.cn", password: "student", wantErr: true},
		{name: "case-sensitive local part comparison", email: "student@henu.edu.cn", password: "Student1234", wantErr: false},
		{name: "versioned weak password", email: "student@henu.edu.cn", password: "PASSWORD123", wantErr: true},
		{name: "leading and trailing spaces are input", email: "student@henu.edu.cn", password: "  passphrase  ", wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.email, test.password)
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate(%q, %q) error = %v, wantErr=%t", test.email, test.password, err, test.wantErr)
			}
		})
	}
}

func TestManagerHashesVerifiesAndRequestsParameterUpgrade(t *testing.T) {
	ctx := context.Background()
	oldParameters := Parameters{MemoryKiB: 32 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	currentParameters := Parameters{MemoryKiB: 32 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	oldManager, err := New(oldParameters, 1)
	if err != nil {
		t.Fatalf("create old password manager: %v", err)
	}
	currentManager, err := New(currentParameters, 1)
	if err != nil {
		t.Fatalf("create current password manager: %v", err)
	}
	verifier, err := oldManager.Hash(ctx, "correct horse 电池 staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if strings.Contains(verifier, "correct horse") || !strings.HasPrefix(verifier, "$argon2id$v=") {
		t.Fatalf("unexpected verifier representation: %q", verifier)
	}
	valid, needsUpgrade, err := currentManager.Verify(ctx, "correct horse 电池 staple", verifier)
	if err != nil || !valid || !needsUpgrade {
		t.Fatalf("verify old parameters = valid:%t upgrade:%t err:%v", valid, needsUpgrade, err)
	}
	valid, _, err = currentManager.Verify(ctx, "wrong password value", verifier)
	if err != nil || valid {
		t.Fatalf("verify wrong password = valid:%t err:%v", valid, err)
	}
}

func TestManagerRejectsParametersOutsideVerificationSafetyBounds(t *testing.T) {
	parameters := DefaultParameters()
	parameters.MemoryKiB = 1024*1024 + 1
	if _, err := New(parameters, 1); err == nil {
		t.Fatal("New accepted a memory cost that Verify refuses")
	}
	parameters = DefaultParameters()
	parameters.Iterations = 11
	if _, err := New(parameters, 1); err == nil {
		t.Fatal("New accepted an iteration count that Verify refuses")
	}
	parameters = DefaultParameters()
	parameters.Parallelism = 17
	if _, err := New(parameters, 1); err == nil {
		t.Fatal("New accepted parallelism that Verify refuses")
	}
}

func TestManagerFailsClosedWhenHashSlotWaitIsCanceled(t *testing.T) {
	manager, err := New(Parameters{
		MemoryKiB: 32 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	}, 1)
	if err != nil {
		t.Fatalf("create password manager: %v", err)
	}
	manager.slots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = manager.Hash(ctx, "correct horse 电池 staple")
	<-manager.slots
	if !errors.Is(err, ErrDependency) {
		t.Fatalf("canceled hash error = %v, want dependency failure", err)
	}
}

func BenchmarkArgon2idDefault(b *testing.B) {
	manager, err := New(DefaultParameters(), 1)
	if err != nil {
		b.Fatalf("create password manager: %v", err)
	}
	for range b.N {
		if _, err := manager.Hash(context.Background(), "representative benchmark passphrase"); err != nil {
			b.Fatalf("hash password: %v", err)
		}
	}
}
