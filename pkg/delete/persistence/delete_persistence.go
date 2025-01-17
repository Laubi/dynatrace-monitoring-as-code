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

package persistence

// FileDefinition represents a loaded YAML delete file consisting of a list of delete entries called 'delete'
// In this struct DeleteEntries may either be a legacy shorthand string or full DeleteEntry value.
// Use FullFileDefinition if you're always working with DeleteEntry values instead
type FileDefinition struct {
	// DeleteEntries loaded from a file are either legacy shorthand strings or full DeleteEntry values
	DeleteEntries []interface{} `yaml:"delete"`
}

// FullFileDefinition represents a delete file consisting of a list of delete entries called 'delete'
// In this struct DeleteEntries are DeleteEntry values.
type FullFileDefinition struct {
	// DeleteEntries loaded from a file are either legacy shorthand strings or full DeleteEntry values
	DeleteEntries []DeleteEntry `yaml:"delete"`
}

// DeleteEntry is a full representation of a delete entry loaded from a YAML delete file
// ConfigId and ConfigName should be mutually exclusive (validated if using LoadEntriesToDelete)
type DeleteEntry struct {
	Project    string `yaml:"project,omitempty" mapstructure:"project"`
	Type       string `yaml:"type" mapstructure:"type"`
	ConfigId   string `yaml:"id,omitempty" mapstructure:"id"`
	ConfigName string `yaml:"name,omitempty" mapstructure:"name"`
}
