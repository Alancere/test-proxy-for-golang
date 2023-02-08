// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.  All rights reserved.
// ------------------------------------------------------------

package main

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type testProxyMode struct {
	RecordMode   string
	PlaybackMode string
	LiveMode     string
}

var TestProxyMode = testProxyMode{
	RecordMode:   "record",
	PlaybackMode: "playback",
	LiveMode:     "live",
}

type testProxyHeader struct {
	RecordingIdHeader          string
	RecordingModeHeader        string
	RecordingUpstreamURIHeader string
}

var TestProxyHeader = testProxyHeader{
	RecordingIdHeader:          "x-recording-id",
	RecordingModeHeader:        "x-recording-mode",
	RecordingUpstreamURIHeader: "x-recording-upstream-base-uri",
}

var client = http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

type TestProxy struct {
	Host          string
	Port          int
	Mode          string
	RecordingId   string
	RecordingPath string
}

func (tpv TestProxy) host() string {
	return fmt.Sprintf("%s:%d", tpv.Host, tpv.Port)
}

func (tpv TestProxy) scheme() string {
	return "https"
}

func (tpv TestProxy) baseURL() string {
	return fmt.Sprintf("https://%s:%d", tpv.Host, tpv.Port)
}

// Do with recording mode.
// When handling live request, the policy will do nothing.
// Otherwise, the policy will replace the URL of the request with the test proxy endpoint.
// After request, the policy will change back to the original URL for the request to prevent wrong polling URL for LRO.
func (tpv *TestProxy) Do(req *policy.Request) (resp *http.Response, err error) {
	oriSchema := req.Raw().URL.Scheme
	oriHost := req.Raw().URL.Host
	req.Raw().URL.Scheme = tpv.scheme()
	req.Raw().URL.Host = tpv.host()
	req.Raw().Host = tpv.host()

	// replace request target to use test proxy
	req.Raw().Header.Set(TestProxyHeader.RecordingUpstreamURIHeader, fmt.Sprintf("%v://%v", oriSchema, oriHost))
	req.Raw().Header.Set(TestProxyHeader.RecordingModeHeader, tpv.Mode)
	req.Raw().Header.Set(TestProxyHeader.RecordingIdHeader, tpv.RecordingId)

	resp, err = req.Next()
	// for any lro operation, need to change back to the original target to prevent
	if resp != nil {
		resp.Request.URL.Scheme = oriSchema
		resp.Request.URL.Host = oriHost
	}
	return resp, err
}

func GetClientOption(tpv *TestProxy, client *http.Client) (*arm.ClientOptions, error) {
	options := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			PerCallPolicies: []policy.Policy{tpv},
			Transport:       client,
		},
	}

	return options, nil
}
