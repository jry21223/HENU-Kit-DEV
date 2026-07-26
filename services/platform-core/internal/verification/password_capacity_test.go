package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"henukit.dev/platform-core/internal/password"
)

type capacityTestCoordinator struct{}

func (capacityTestCoordinator) Allow(context.Context, string, int64, time.Duration) (bool, error) {
	return true, nil
}

func (capacityTestCoordinator) FailureCount(context.Context, string) (int64, error) {
	return 0, nil
}

func (capacityTestCoordinator) RecordFailure(context.Context, string, time.Duration) (int64, error) {
	return 1, nil
}

func (capacityTestCoordinator) Clear(context.Context, ...string) error {
	return nil
}

func TestPasswordLoginRejectsSaturatedArgon2BeforeOpeningDatabaseTransaction(t *testing.T) {
	manager, err := password.New(password.Parameters{
		MemoryKiB: 32 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	}, 1)
	if err != nil {
		t.Fatalf("create password manager: %v", err)
	}
	reservation, err := manager.TryReserve()
	if err != nil {
		t.Fatalf("occupy Argon2 capacity: %v", err)
	}
	defer reservation.Release()

	service := &Service{
		coordinator:    capacityTestCoordinator{},
		secretKey:      make([]byte, 32),
		allowedDomains: map[string]struct{}{"henu.edu.cn": {}},
		passwords:      manager,
	}
	_, err = service.PasswordLogin(context.Background(), PasswordLoginInput{
		Email: "student@henu.edu.cn", Password: "correct horse 电池 staple",
		DeviceID: "capacity-test-device", ClientIP: "127.0.0.1",
	})
	if !errors.Is(err, ErrDependency) {
		t.Fatalf("saturated password login error = %v, want dependency failure", err)
	}
}
