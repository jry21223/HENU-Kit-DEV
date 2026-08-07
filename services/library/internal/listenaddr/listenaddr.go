// Package listenaddr holds the library service's default listen address,
// shared by the server and healthcheck entrypoints so the two can never
// drift apart.
package listenaddr

// DefaultAddr is the address the service binds when LIBRARY_ADDR is unset.
// docker-compose.henukit.yml mirrors this value in LIBRARY_ADDR:-:8095 for
// the source-build path; the prebuilt path (docker-compose.henukit.prebuilt.yml)
// never sets LIBRARY_ADDR, so this constant is the actual binding port there.
// Keep it in sync with the compose default.
const DefaultAddr = ":8095"
