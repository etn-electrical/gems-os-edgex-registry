//
// Copyright (C) 2024 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package keeper

import (
	"net/http"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/interfaces"
)

type emptyAuthenticationInjector struct {
}

// NewNullAuthenticationInjector creates an instance of AuthenticationInjector
func NewNullAuthenticationInjector() interfaces.AuthenticationInjector {
	return &emptyAuthenticationInjector{}
}

func (_ *emptyAuthenticationInjector) AddAuthenticationData(_ *http.Request) error {
	// Do nothing to the request; used for unit tests
	return nil
}

func (_ *emptyAuthenticationInjector) RoundTripper() http.RoundTripper {
	// Do nothing to the request; used for unit tests
	return nil
}
