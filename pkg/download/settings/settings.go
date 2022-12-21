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

package settings

import (
	config "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/coordinate"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/parameter"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/parameter/value"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/template"
	v2 "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/project/v2"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/rest"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/log"
)

func Download(client rest.SettingsClient) v2.ConfigsPerType {

	log.Debug("Fetching schemas to download")
	schemas, err := client.ListSchemas()
	if err != nil {
		log.Error("Failed to fetch all known schemas. Skipping settings download. Reason: %s", err)
		return nil
	}

	results := make(v2.ConfigsPerType)

	for _, schema := range schemas {
		log.Debug("Downloading all settings for schema %s", schema)
		objects, err := client.ListSettings(schema.SchemaId)
		if err != nil {
			log.Error("Failed to fetch all settings for schema %s", schema)
			continue
		}

		configs := convertAllObjects(objects)
		results[schema.SchemaId] = configs
	}

	return results
}

func convertAllObjects(objects []rest.DownloadSettingsObject) []config.Config {
	result := make([]config.Config, 0, len(objects))

	for _, o := range objects {
		result = append(result, convertObject(o))
	}

	return result
}

func convertObject(o rest.DownloadSettingsObject) config.Config {

	content := string(*o.Value)

	templ := template.NewDownloadTemplate(o.ObjectId, o.ObjectId, content)

	return config.Config{
		Template: templ,
		Coordinate: coordinate.Coordinate{
			Project:  "project",
			Type:     o.SchemaId,
			ConfigId: o.ObjectId, // we need a unique id -> use the objectId as it is unique
		},
		Type: config.Type{
			SchemaId:      o.SchemaId,
			SchemaVersion: o.SchemaVersion,
		},
		Parameters: map[string]parameter.Parameter{
			config.NameParameter:  &value.ValueParameter{Value: o.ObjectId},
			config.ScopeParameter: &value.ValueParameter{Value: o.Scope},
		},
		References: nil,
		Skip:       false,
	}

}