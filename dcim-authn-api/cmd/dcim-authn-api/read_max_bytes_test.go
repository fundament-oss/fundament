package main

import (
	"net/http"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/dcim-authn-api/pkg/dcimauthn"
	"github.com/fundament-oss/fundament/dcim-authn-api/pkg/proto/gen/dcimauthn/v1/dcimauthnv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	mux := http.NewServeMux()
	registerRPCHandlers(mux, &dcimauthn.Server{})

	connectlimittest.AssertRejectsGzipBomb(t, mux, dcimauthnv1connect.AuthnServiceGetUserInfoProcedure)
}
