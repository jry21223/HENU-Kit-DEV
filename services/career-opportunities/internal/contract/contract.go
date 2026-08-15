// Package contract holds the Career service route constants. They mirror the
// route strings the worker and HTTP tests assert against.
package contract

const (
	HealthRoute       = "/healthz"
	CreateSearchRoute = "/api/v1/career/searches"
	SearchRoute       = "/api/v1/career/searches/{id}"
	ListSearchesRoute = "/api/v1/career/searches"
)
