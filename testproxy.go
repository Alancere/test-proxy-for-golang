// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.  All rights reserved.
// ------------------------------------------------------------

package testproxy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"testing"

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
	Client *http.Client
	Host          string
	Port          int
	Mode          string
	RecordingId   string
	RecordingPath string
}

func NewTestProxy() *TestProxy {
	return &TestProxy{
		Client: &client,
	}
}

func (tp TestProxy) host() string {
	return fmt.Sprintf("%s:%d", tp.Host, tp.Port)
}

func (tp TestProxy) scheme() string {
	if tp.Port == 5001 {
		return "https"
	}
	return "http"
}

func (tp TestProxy) baseURL() string {
	return fmt.Sprintf("%s://%s:%d", tp.scheme(),tp.Host, tp.Port)
}

func (tp *TestProxy)Do(req *http.Request) (resp *http.Response, err error) {
	oriSchema := req.URL.Scheme
	oriHost := req.URL.Host
	req.URL.Scheme = tp.scheme()
	req.URL.Host = tp.host()
	req.Host = tp.host()

	// replace request target to use test proxy
	req.Header.Set(TestProxyHeader.RecordingUpstreamURIHeader, fmt.Sprintf("%v://%v", oriSchema, oriHost))
	req.Header.Set(TestProxyHeader.RecordingModeHeader, tp.Mode)
	req.Header.Set(TestProxyHeader.RecordingIdHeader, tp.RecordingId)

	resp,err = tp.Client.Do(req)

	// // for any lro operation, need to change back to the original target to prevent
	if resp != nil {
		resp.Request.URL.Scheme = oriSchema
		resp.Request.URL.Host = oriHost
	}

	return resp, err
}

func GetClientOption(tp *TestProxy) (*arm.ClientOptions, error) {
	options := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport:       tp,
		},
	}

	return options, nil
}

func GetCurrentDirectory() string {
	root, err := filepath.Abs(".")
	if err != nil {
		log.Fatal(err)
	}
	return root
}

func getRecordingFilePath(recordingPath string, t *testing.T) string {
	return path.Join(recordingPath, "recordings", t.Name()+".json")
}

// StartTestProxy tells the test proxy to begin accepting requests for a given test
func StartTestProxy(t *testing.T, tp *TestProxy) error {
	if tp == nil {
		return fmt.Errorf("TestProxy not empty")
	}

	recordingFilePath := getRecordingFilePath(tp.RecordingPath, t)
	url := fmt.Sprintf("%s/%s/start", tp.baseURL(), tp.Mode)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	marshalled, err := json.Marshal(map[string]string{"x-recording-file": recordingFilePath})
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(marshalled))
	req.ContentLength = int64(len(marshalled))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	recId := resp.Header.Get(TestProxyHeader.RecordingIdHeader)
	if recId == "" {
		b, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return err
		}
		return fmt.Errorf("recording ID was not returned by the response. Response body: %s", b)
	}

	tp.RecordingId = recId

	// Unmarshal any variables returned by the proxy
	var m map[string]interface{}
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if len(body) > 0 {
		err = json.Unmarshal(body, &m)
		if err != nil {
			return err
		}
	}

	return nil
}

// StopTestProxy tells the test proxy to stop accepting requests for a given test
func StopTestProxy(t *testing.T, tp *TestProxy) error {
	if tp == nil {
		return fmt.Errorf("TestProxy not empty")
	}

	url := fmt.Sprintf("%v/%v/stop", tp.baseURL(), tp.Mode)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set(TestProxyHeader.RecordingIdHeader, tp.RecordingId)

	resp, err := client.Do(req)
	if resp.StatusCode != 200 {
		b, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err == nil {
			return fmt.Errorf("proxy did not stop the recording properly: %s", string(b))
		}
		return fmt.Errorf("proxy did not stop the recording properly: %s", err.Error())
	}
	_ = resp.Body.Close()
	return err
}
