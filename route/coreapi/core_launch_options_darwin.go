//go:build darwin

package coreapi

import (
	"net/http"
	corepkg "routex-service/core"
	"routex-service/route/pipectx"
)

func coreLaunchOptions(r *http.Request) []corepkg.LaunchOption {
	info, ok := pipectx.RequestDarwinPeerInfo(r)
	if !ok || !info.HasGID {
		return nil
	}
	return []corepkg.LaunchOption{corepkg.WithLogFileGroup(info.GID)}
}
