//go:build integration
// +build integration

/**
 * @license
 * Copyright 2020 Dynatrace LLC
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v2

import (
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/cmd/monaco/runner"
	"github.com/spf13/afero"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestAllDuplicateErrorsAreReported(t *testing.T) {

	configFolder := "test-resources/configs-with-duplicate-ids/"
	manifest := filepath.Join(configFolder, "manifest.yaml")

	RunIntegrationWithCleanup(t, configFolder, manifest, "", "DuplicateIdErrors", func(fs afero.Fs) {

		cmd := runner.BuildCli(fs)
		cmd.SetArgs([]string{
			"deploy",
			"--verbose",
			manifest,
		})

		output := strings.Builder{}
		cmd.SetOut(&output)
		err := cmd.Execute()

		assert.ErrorContains(t, err, "error while loading projects")

		runLog := strings.ToLower(output.String())
		strings.Contains(runLog, "duplicate")
		strings.Contains(runLog, "project:alerting-profile:profile")
	})
}