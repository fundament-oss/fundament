package main

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/fundament-oss/fundament/authn-api/pkg/authn"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	mux := http.NewServeMux()
	registerRPCHandlers(mux, &authn.AuthnServer{}, slog.New(slog.DiscardHandler))

	connectlimittest.AssertRejectsGzipBomb(t, mux, authnv1connect.AuthnServiceGetUserInfoProcedure)
	connectlimittest.AssertRejectsGzipBomb(t, mux, authnv1connect.TokenServiceExchangeTokenProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, mux)
}
