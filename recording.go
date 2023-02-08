// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.  All rights reserved.
// ------------------------------------------------------------

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func GetCurrentDirectory() string {
	root, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	return strings.ReplaceAll(root,"\\","/")
}

func getRecordingFilePath(recordingPath string, t *testing.T) string {
	return path.Join(recordingPath, "recordings", t.Name()+".json")
}

// StartTestProxy tells the test proxy to begin accepting requests for a given test
func StartTestProxy(t *testing.T, tpv *TestProxy) error {
	if tpv == nil {
		return fmt.Errorf("TestProxy not empty")
	}

	recordingFilePath := getRecordingFilePath(tpv.RecordingPath, t)
	url := fmt.Sprintf("%s/%s/start", tpv.baseURL(), tpv.Mode)

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

	tpv.RecordingId = recId

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
func StopTestProxy(t *testing.T, tpv *TestProxy) error {
	if tpv == nil {
		return fmt.Errorf("TestProxy not empty")
	}

	url := fmt.Sprintf("%v/%v/stop", tpv.baseURL(), tpv.Mode)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set(TestProxyHeader.RecordingIdHeader, tpv.RecordingId)

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
