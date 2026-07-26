package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const PolicyVersion = 1

var (
	ErrInvalid    = errors.New("password does not satisfy policy")
	ErrDependency = errors.New("password dependency unavailable")
)

type Parameters struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func DefaultParameters() Parameters {
	return Parameters{
		MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 1,
		SaltLength: 16, KeyLength: 32,
	}
}

type Manager struct {
	parameters Parameters
	slots      chan struct{}
	random     io.Reader
}

// Reservation owns one Argon2 capacity slot. Password authentication reserves
// capacity before opening a database transaction so requests cannot wait for
// memory while holding scarce database connections.
type Reservation struct {
	manager *Manager
	once    sync.Once
}

func New(parameters Parameters, maxConcurrent int) (*Manager, error) {
	if parameters.MemoryKiB < 32*1024 || parameters.Iterations < 1 || parameters.Parallelism < 1 ||
		parameters.SaltLength < 16 || parameters.KeyLength < 32 || maxConcurrent < 1 ||
		parameters.MemoryKiB > 1024*1024 || parameters.Iterations > 10 || parameters.Parallelism > 16 ||
		parameters.SaltLength > 64 || parameters.KeyLength > 128 {
		return nil, errors.New("secure Argon2id parameters and a positive concurrency limit are required")
	}
	return &Manager{parameters: parameters, slots: make(chan struct{}, maxConcurrent), random: rand.Reader}, nil
}

func Validate(email, value string) error {
	if !AcceptableInput(value) {
		return ErrInvalid
	}
	localPart, _, ok := strings.Cut(email, "@")
	if !ok || value == localPart {
		return ErrInvalid
	}
	if _, weak := weakPasswords[strings.ToLower(value)]; weak {
		return ErrInvalid
	}
	return nil
}

func AcceptableInput(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	length := utf8.RuneCountInString(value)
	return length >= 10 && length <= 128
}

func (m *Manager) Hash(ctx context.Context, value string) (string, error) {
	if err := m.acquire(ctx); err != nil {
		return "", err
	}
	defer m.release()
	salt := make([]byte, m.parameters.SaltLength)
	if _, err := io.ReadFull(m.random, salt); err != nil {
		return "", ErrDependency
	}
	hash := argon2.IDKey([]byte(value), salt, m.parameters.Iterations, m.parameters.MemoryKiB, m.parameters.Parallelism, m.parameters.KeyLength)
	return encode(m.parameters, salt, hash), nil
}

func (m *Manager) Verify(ctx context.Context, value, verifier string) (bool, bool, error) {
	parameters, salt, expected, err := decode(verifier)
	if err != nil {
		return false, false, ErrDependency
	}
	if err := m.acquire(ctx); err != nil {
		return false, false, err
	}
	defer m.release()
	actual := argon2.IDKey([]byte(value), salt, parameters.Iterations, parameters.MemoryKiB, parameters.Parallelism, uint32(len(expected)))
	valid := subtle.ConstantTimeCompare(actual, expected) == 1
	return valid, valid && parameters != m.parameters, nil
}

// TryReserve acquires Argon2 capacity without queuing. Callers that would hold
// another scarce dependency while hashing must fail closed when capacity is
// already saturated.
func (m *Manager) TryReserve() (*Reservation, error) {
	select {
	case m.slots <- struct{}{}:
		return &Reservation{manager: m}, nil
	default:
		return nil, ErrDependency
	}
}

func (reservation *Reservation) Hash(value string) (string, error) {
	if reservation == nil || reservation.manager == nil {
		return "", ErrDependency
	}
	salt := make([]byte, reservation.manager.parameters.SaltLength)
	if _, err := io.ReadFull(reservation.manager.random, salt); err != nil {
		return "", ErrDependency
	}
	hash := argon2.IDKey(
		[]byte(value),
		salt,
		reservation.manager.parameters.Iterations,
		reservation.manager.parameters.MemoryKiB,
		reservation.manager.parameters.Parallelism,
		reservation.manager.parameters.KeyLength,
	)
	return encode(reservation.manager.parameters, salt, hash), nil
}

func (reservation *Reservation) Verify(value, verifier string) (bool, bool, error) {
	if reservation == nil || reservation.manager == nil {
		return false, false, ErrDependency
	}
	parameters, salt, expected, err := decode(verifier)
	if err != nil {
		return false, false, ErrDependency
	}
	actual := argon2.IDKey([]byte(value), salt, parameters.Iterations, parameters.MemoryKiB, parameters.Parallelism, uint32(len(expected)))
	valid := subtle.ConstantTimeCompare(actual, expected) == 1
	return valid, valid && parameters != reservation.manager.parameters, nil
}

func (reservation *Reservation) Release() {
	if reservation == nil || reservation.manager == nil {
		return
	}
	reservation.once.Do(reservation.manager.release)
}

func (m *Manager) acquire(ctx context.Context) error {
	select {
	case m.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ErrDependency
	}
}

func (m *Manager) release() {
	<-m.slots
}

func encode(parameters Parameters, salt, hash []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, parameters.MemoryKiB, parameters.Iterations, parameters.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}

func decode(value string) (Parameters, []byte, []byte, error) {
	parts := strings.Split(value, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return Parameters{}, nil, nil, ErrDependency
	}
	if len(parts[4]) > 88 || len(parts[5]) > 172 {
		return Parameters{}, nil, nil, ErrDependency
	}
	var parameters Parameters
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.MemoryKiB, &parameters.Iterations, &parameters.Parallelism); err != nil {
		return Parameters{}, nil, nil, ErrDependency
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Parameters{}, nil, nil, ErrDependency
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(salt) < 16 || len(hash) < 32 || len(salt) > 64 || len(hash) > 128 {
		return Parameters{}, nil, nil, ErrDependency
	}
	parameters.SaltLength, parameters.KeyLength = uint32(len(salt)), uint32(len(hash))
	if parameters.MemoryKiB < 32*1024 || parameters.Iterations < 1 || parameters.Parallelism < 1 {
		return Parameters{}, nil, nil, ErrDependency
	}
	if parameters.MemoryKiB > 1024*1024 || parameters.Iterations > 10 || parameters.Parallelism > 16 {
		return Parameters{}, nil, nil, ErrDependency
	}
	return parameters, salt, hash, nil
}

var weakPasswords = map[string]struct{}{
	"1234567890": {}, "1111111111": {}, "password123": {}, "qwerty12345": {},
	"123456789a": {}, "abcdefghij": {}, "iloveyou123": {}, "adminadmin": {},
}
