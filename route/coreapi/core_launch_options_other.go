//go:build !linux && !darwin

package coreapi

import (
	"net/http"
	corepkg "routex-service/core"
)

func coreLaunchOptions(_ *http.Request) []corepkg.LaunchOption {
	return nil
}
