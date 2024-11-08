//
// Copyright (C) 2022 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/edgexfoundry/go-mod-registry/v2/pkg/types"
)

const defaultTimeout = 10 * time.Second

type keeperClient struct {
	config              *types.Config
	keeperUrl           string
	serviceKey          string
	serviceHost         string
	servicePort         int
	healthCheckRoute    string
	healthCheckInterval string
}

func NewKeeperClient(registryConfig types.Config) (*keeperClient, error) {
	client := keeperClient{
		config:     &registryConfig,
		serviceKey: registryConfig.ServiceKey,
		keeperUrl:  registryConfig.GetRegistryUrl(),
	}

	// ServiceHost will be empty when client isn't registering the service
	if registryConfig.ServiceHost != "" {
		client.servicePort = registryConfig.ServicePort
		client.serviceHost = registryConfig.ServiceHost
		client.healthCheckRoute = registryConfig.CheckRoute
		client.healthCheckInterval = registryConfig.CheckInterval
	}

	return &client, nil
}

func (k *keeperClient) Register() error {
	if k.serviceKey == "" || k.serviceHost == "" || k.servicePort == 0 ||
		k.healthCheckRoute == "" || k.healthCheckInterval == "" {
		return fmt.Errorf("unable to register service with keeper: Service information not set")
	}

	registrationReq := AddRegistrationRequest{
		BaseRequest: BaseRequest{
			Versionable: Versionable{ApiVersion: ApiVersion},
		},
		Registration: RegistrationDTO{
			ServiceId: k.serviceKey,
			Host:      k.serviceHost,
			Port:      k.servicePort,
			HealthCheck: HealthCheck{
				Interval: k.healthCheckInterval,
				Path:     k.healthCheckRoute,
				Type:     "http",
			},
		},
	}

	jsonEncodedData, err := json.Marshal(registrationReq)
	if err != nil {
		return fmt.Errorf("failed to encode registration request: %s", err.Error())
	}

	// check if the service registry exists first
	resp, err := getRegistryByService(k.config.GetRegistryUrl() + ApiRegistrationByServiceIdRoute + k.serviceKey)
	if err != nil {
		return fmt.Errorf("failed to check the %s service registry status: %s", k.serviceKey, err.Error())
	}

	// call the PUT registry API to update the registry if the service already exists
	// otherwise, call the POST API to create the registry
	httpMethod := http.MethodPost
	if resp.StatusCode == http.StatusOK {
		httpMethod = http.MethodPut
	}
	req, err := http.NewRequest(httpMethod, k.config.GetRegistryUrl()+ApiRegisterRoute, bytes.NewReader(jsonEncodedData))
	if err != nil {
		return fmt.Errorf("failed to create register request: %s", err.Error())
	}
	req.Header.Set(ContentType, ContentTypeJSON)

	client := http.Client{Timeout: defaultTimeout}
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		var response BaseResponse
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %s", err.Error())
		}
		err = json.Unmarshal(bodyBytes, &response)
		if err != nil {
			return fmt.Errorf("failed to decode response body: %s", err.Error())
		}
		return fmt.Errorf("failed to register %s: %s", k.serviceKey, response.Message)
	}

	return nil
}

func (k *keeperClient) Unregister() error {
	req, err := http.NewRequest(http.MethodDelete, k.config.GetRegistryUrl()+ApiRegistrationByServiceIdRoute+k.serviceKey, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create unregister request: %s", err.Error())
	}

	client := http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		var response BaseResponse
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %s", err.Error())
		}
		err = json.Unmarshal(bodyBytes, &response)
		if err != nil {
			return fmt.Errorf("failed to decode response body: %s", err.Error())
		}
		return fmt.Errorf("failed to unregister %s: %s", k.serviceKey, response.Message)
	}

	return nil
}

func (k *keeperClient) RegisterCheck(id string, name string, notes string, url string, interval string) error {
	// keeper combines service discovery and health check into one single register request
	return nil
}

func (k *keeperClient) IsAlive() bool {
	client := http.Client{Timeout: defaultTimeout}
	resp, err := client.Get(k.keeperUrl + ApiPingRoute)
	if err != nil {
		return false
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return true
	}

	return false
}

func (k *keeperClient) GetServiceEndpoint(serviceKey string) (types.ServiceEndpoint, error) {
	req, err := http.NewRequest(http.MethodGet, k.config.GetRegistryUrl()+ApiRegistrationByServiceIdRoute+serviceKey, http.NoBody)
	if err != nil {
		return types.ServiceEndpoint{}, fmt.Errorf("failed to create http request: %s", err.Error())
	}

	client := http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return types.ServiceEndpoint{}, fmt.Errorf("http error: %s", err.Error())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.ServiceEndpoint{}, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		var response BaseResponse
		err = json.Unmarshal(bodyBytes, &response)
		if err != nil {
			return types.ServiceEndpoint{}, fmt.Errorf("failed to decode response body: %s", err.Error())
		}
		return types.ServiceEndpoint{}, fmt.Errorf("failed to get service endpoint: %s", response.Message)
	}

	var r RegistrationResponse
	err = json.Unmarshal(bodyBytes, &r)
	if err != nil {
		return types.ServiceEndpoint{}, fmt.Errorf("failed to decode response body: %s", err.Error())
	}

	endpoint := types.ServiceEndpoint{
		ServiceId: serviceKey,
		Host:      r.Registration.Host,
		Port:      r.Registration.Port,
	}

	return endpoint, nil
}

func (k *keeperClient) GetAllServiceEndpoints() ([]types.ServiceEndpoint, error) {
	req, err := http.NewRequest(http.MethodGet, k.config.GetRegistryUrl()+ApiAllRegistrationRoute, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %s", err.Error())
	}

	client := http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http error: %s", err.Error())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		var response BaseResponse
		err = json.Unmarshal(bodyBytes, &response)
		if err != nil {
			return nil, fmt.Errorf("failed to decode response body: %s", err.Error())
		}
		return nil, fmt.Errorf("failed to get all service endpoints: %s", response.Message)

	}

	var responseDTO MultiRegistrationsResponse
	err = json.Unmarshal(bodyBytes, &responseDTO)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response body: %s", err.Error())
	}

	endpoints := make([]types.ServiceEndpoint, len(responseDTO.Registrations))
	for idx, r := range responseDTO.Registrations {
		endpoint := types.ServiceEndpoint{
			ServiceId: r.ServiceId,
			Host:      r.Host,
			Port:      r.Port,
		}
		endpoints[idx] = endpoint
	}

	return endpoints, nil
}

// getRegistryByService invokes the GET registry by service API and returns the response
func getRegistryByService(registryUrl string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, registryUrl, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %s", err.Error())
	}

	client := http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http error: %s", err.Error())
	}

	return resp, nil
}

func (k *keeperClient) IsServiceAvailable(serviceKey string) (bool, error) {
	resp, err := getRegistryByService(k.config.GetRegistryUrl() + ApiRegistrationByServiceIdRoute + serviceKey)
	if err != nil {
		return false, fmt.Errorf("failed to get %s service registry: %s", serviceKey, err.Error())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var response RegistrationResponse
		err = json.Unmarshal(bodyBytes, &response)
		if err != nil {
			return false, fmt.Errorf("failed to decode response body: %s", err.Error())
		}

		if !strings.EqualFold(response.Registration.Status, "up") {
			return false, fmt.Errorf(" %s service not healthy...", serviceKey)
		}

		return true, nil
	case http.StatusNotFound:
		return false, fmt.Errorf("%s service is not registered. Might not have started... ", serviceKey)
	default:
		var response BaseResponse
		err = json.Unmarshal(bodyBytes, &response)
		if err != nil {
			return false, fmt.Errorf("failed to decode response body: %s", err.Error())
		}
		return false, fmt.Errorf("failed to check service availability: %s", response.Message)
	}
}
