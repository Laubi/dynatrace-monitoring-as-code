//go:build unit

/*
 * @license
 * Copyright 2023 Dynatrace LLC
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

package v1

import (
	"github.com/dynatrace/dynatrace-configuration-as-code/internal/template"
	"github.com/dynatrace/dynatrace-configuration-as-code/pkg/api"
)

func NewConfigWithTemplate(id string, project string, fileName string, template template.Template, properties map[string]map[string]string, api api.Api) *Config {
	return newConfigWithTemplate(id, project, template, filterProperties(id, properties), api, fileName)
}