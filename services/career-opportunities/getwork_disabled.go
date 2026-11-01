package career

import (
	"context"
	"errors"
)

// ErrJobSourceDisabled is distinct from a scan that completed without matches.
var ErrJobSourceDisabled = errors.New("career job source is disabled")

const jobSourceDisabledMessage = "岗位扫描暂未启用，请稍后再试"

// DisabledJobSourceWork rejects retained queued scans without producing an
// empty successful result or an Opportunity Digest.
func DisabledJobSourceWork(context.Context, any) (WorkResult, error) {
	return WorkResult{}, ErrJobSourceDisabled
}
