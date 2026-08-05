// food-healthcheck is a small static health probe for the distroless
// production image. It intentionally verifies the service's own /healthz
// route instead of merely proving that a TCP port opened.
package main

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	address := strings.TrimSpace(os.Getenv("FOOD_ADDR"))
	if address == "" {
		address = ":8096"
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		os.Exit(2)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
