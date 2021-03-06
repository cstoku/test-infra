/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package initupload

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

func (o Options) Run() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	var cloneRecords []clone.Record
	data, err := ioutil.ReadFile(o.Log)
	if err != nil {
		return fmt.Errorf("could not read clone log: %v", err)
	}
	if err = json.Unmarshal(data, &cloneRecords); err != nil {
		return fmt.Errorf("could not unmarshal clone records: %v", err)
	}

	// Do not read from cloneLog directly.
	// Instead create multiple readers from cloneLog so it can be uploaded to
	// both clone-log.txt and build-log.txt on failure.
	cloneLog := bytes.Buffer{}
	failed := false
	for _, record := range cloneRecords {
		cloneLog.WriteString(clone.FormatRecord(record))
		failed = failed || record.Failed
	}

	uploadTargets := map[string]gcs.UploadFunc{
		"clone-log.txt":      gcs.DataUpload(bytes.NewReader(cloneLog.Bytes())),
		"clone-records.json": gcs.FileUpload(o.Log),
	}

	started := struct {
		Timestamp int64 `json:"timestamp"`
	}{
		Timestamp: time.Now().Unix(),
	}
	startedData, err := json.Marshal(&started)
	if err != nil {
		return fmt.Errorf("could not marshal starting data: %v", err)
	} else {
		uploadTargets["started.json"] = gcs.DataUpload(bytes.NewReader(startedData))
	}

	if failed {
		finished := struct {
			Timestamp int64  `json:"timestamp"`
			Passed    bool   `json:"passed"`
			Result    string `json:"result"`
		}{
			Timestamp: time.Now().Unix(),
			Passed:    false,
			Result:    "FAILURE",
		}
		finishedData, err := json.Marshal(&finished)
		if err != nil {
			return fmt.Errorf("could not marshal finishing data: %v", err)
		} else {
			uploadTargets["build-log.txt"] = gcs.DataUpload(bytes.NewReader(cloneLog.Bytes()))
			uploadTargets["finished.json"] = gcs.DataUpload(bytes.NewReader(finishedData))
		}
	}

	if err := o.Options.Run(spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to GCS: %v", err)
	}

	if failed {
		return errors.New("cloning the appropriate refs failed")
	}

	return nil
}
