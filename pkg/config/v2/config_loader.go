// @license
// Copyright 2021 Dynatrace LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v2

import (
	"fmt"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/maps"
	"path/filepath"
	"strings"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/coordinate"
	configErrors "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/errors"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/parameter"
	refParam "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/parameter/reference"
	valueParam "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/parameter/value"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/template"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/manifest"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/files"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v2"
)

type LoaderContext struct {
	ProjectId       string
	Path            string
	Environments    []manifest.EnvironmentDefinition
	KnownApis       map[string]struct{}
	ParametersSerDe map[string]parameter.ParameterSerDe
}

// LoadConfigs will search a given path for configuration yamls and parses them.
// It will try to parse all configurations it finds and returns a list of parsed
// configs. If any error was encountered, the list of configs will be nil and
// only the error slice will be filled.
func LoadConfigs(fs afero.Fs, context *LoaderContext) (result []Config, errors []error) {
	filesInFolder, err := afero.ReadDir(fs, context.Path)

	if err != nil {
		return nil, []error{err}
	}

	for _, file := range filesInFolder {
		filename := file.Name()

		if file.IsDir() || !files.IsYamlFileExtension(filename) {
			continue
		}

		configs, configErrors := parseConfigs(fs, context, filepath.Join(context.Path, filename))

		if configErrors != nil {
			errors = append(errors, configErrors...)
			continue
		}

		result = append(result, configs...)

	}

	if errors != nil {
		return nil, errors
	}

	return result, nil
}

type ConfigLoaderContext struct {
	*LoaderContext
	Folder string
	Path   string
	ApiId  string
}

type DefinitionParserError struct {
	Location coordinate.Coordinate
	Path     string
	Reason   string
}

func newDefinitionParserError(configId string, context *ConfigLoaderContext, reason string) DefinitionParserError {
	return DefinitionParserError{
		Location: coordinate.Coordinate{
			Project: context.ProjectId,
			Api:     context.ApiId,
			Config:  configId,
		},
		Path:   context.Path,
		Reason: reason,
	}
}

type DetailedDefinitionParserError struct {
	DefinitionParserError
	EnvironmentDetails configErrors.EnvironmentDetails
}

func newDetailedDefinitionParserError(configId string, context *ConfigLoaderContext, environment manifest.EnvironmentDefinition,
	reason string) DetailedDefinitionParserError {

	return DetailedDefinitionParserError{
		DefinitionParserError: newDefinitionParserError(configId, context, reason),
		EnvironmentDetails:    configErrors.EnvironmentDetails{Group: environment.Group, Environment: environment.Name},
	}
}

func (e DetailedDefinitionParserError) LocationDetails() configErrors.EnvironmentDetails {
	return e.EnvironmentDetails
}

func (e DetailedDefinitionParserError) Environment() string {
	return e.EnvironmentDetails.Environment
}

func (e DefinitionParserError) Coordinates() coordinate.Coordinate {
	return e.Location
}

type ParameterDefinitionParserError struct {
	DetailedDefinitionParserError
	ParameterName string
}

func newParameterDefinitionParserError(name string, configId string, context *ConfigLoaderContext,
	environment manifest.EnvironmentDefinition, reason string) ParameterDefinitionParserError {

	return ParameterDefinitionParserError{
		DetailedDefinitionParserError: newDetailedDefinitionParserError(configId, context, environment, reason),
		ParameterName:                 name,
	}
}

var (
	_ configErrors.ConfigError         = (*DefinitionParserError)(nil)
	_ configErrors.DetailedConfigError = (*DetailedDefinitionParserError)(nil)
	_ configErrors.DetailedConfigError = (*ParameterDefinitionParserError)(nil)
)

func (e ParameterDefinitionParserError) Error() string {
	return fmt.Sprintf("%s: cannot parse parameter definition in `%s`: %s",
		e.ParameterName, e.Path, e.Reason)
}

func (e DefinitionParserError) Error() string {
	return fmt.Sprintf("cannot parse definition in `%s`: %s",
		e.Path, e.Reason)
}

func parseConfigs(fs afero.Fs, context *LoaderContext, filePath string) (configs []Config, errors []error) {
	data, err := afero.ReadFile(fs, filePath)
	folder := filepath.Dir(filePath)

	if err != nil {
		return nil, []error{err}
	}

	definition := topLevelDefinition{}

	err = yaml.UnmarshalStrict(data, &definition)

	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("field config not found in type %s", getTopLevelDefinitionYamlTypeName())) {
			return nil, []error{
				fmt.Errorf("config '%s' is not valid v2 configuration - you may be loading v1 configs, please 'convert' to v2:\n%w", filePath, err),
			}
		}

		return nil, []error{
			fmt.Errorf("failed to load config '%s':\n%w", filePath, err),
		}
	}

	if len(definition.Configs) == 0 {
		return nil, []error{
			fmt.Errorf("no configurations found in file '%s'", filePath),
		}
	}

	configLoaderContext := &ConfigLoaderContext{
		LoaderContext: context,
		Folder:        folder,
		Path:          filePath,
	}

	for _, config := range definition.Configs {
		result, definitionErrors := parseDefinition(fs, configLoaderContext, config.Id, config)

		if definitionErrors != nil {
			errors = append(errors, definitionErrors...)
			continue
		}

		configs = append(configs, result...)
	}

	if errors != nil {
		return nil, errors
	}

	return configs, nil
}

func parseDefinition(fs afero.Fs, context *ConfigLoaderContext,
	configId string, definition topLevelConfigDefinition) ([]Config, []error) {

	results := make([]Config, 0)
	var errors []error

	if definition.Type.Api == "" {
		return nil, []error{fmt.Errorf("missing config-property type.api in %v", context.Path)}
	}

	if _, found := context.KnownApis[definition.Type.Api]; !found {
		return nil, []error{fmt.Errorf("unknown API: '%v' in %v", definition.Type.Api, context.Path)}
	}

	context.ApiId = definition.Type.Api

	groupOverrideMap := toGroupOverrideMap(definition.GroupOverrides)
	environmentOverrideMap := toEnvironmentOverrideMap(definition.EnvironmentOverrides)

	for _, environment := range context.Environments {
		result, definitionErrors := parseDefinitionForEnvironment(fs, context, configId, environment,
			definition, groupOverrideMap, environmentOverrideMap)

		if definitionErrors != nil {
			errors = append(errors, definitionErrors...)
			continue
		}

		results = append(results, result)
	}

	if errors != nil {
		return nil, errors
	}

	return results, nil
}

func toEnvironmentOverrideMap(environments []environmentOverride) map[string]environmentOverride {
	result := make(map[string]environmentOverride)

	for _, env := range environments {
		result[env.Environment] = env
	}

	return result
}

func toGroupOverrideMap(groups []groupOverride) map[string]groupOverride {
	result := make(map[string]groupOverride)

	for _, group := range groups {
		result[group.Group] = group
	}

	return result
}

func parseDefinitionForEnvironment(fs afero.Fs, context *ConfigLoaderContext,
	configId string, environment manifest.EnvironmentDefinition,
	definition topLevelConfigDefinition, groupOverrides map[string]groupOverride,
	environmentOverride map[string]environmentOverride) (Config, []error) {

	configDefinition := configDefinition{
		Parameters: make(map[string]configParameter),
	}

	applyOverrides(&configDefinition, definition.Config)

	if override, found := groupOverrides[environment.Group]; found {
		applyOverrides(&configDefinition, override.Override)
	}

	if override, found := environmentOverride[environment.Name]; found {
		applyOverrides(&configDefinition, override.Override)
	}

	configDefinition.Template = filepath.FromSlash(configDefinition.Template)

	return getConfigFromDefinition(fs, context, configId, environment, configDefinition)
}

func applyOverrides(base *configDefinition, override configDefinition) {
	if override.Name != nil {
		base.Name = override.Name
	}

	if override.Template != "" {
		base.Template = override.Template
	}

	if override.Skip != nil {
		base.Skip = override.Skip
	}

	for name, param := range override.Parameters {
		base.Parameters[name] = param
	}

}

func getConfigFromDefinition(fs afero.Fs, context *ConfigLoaderContext,
	configId string, environment manifest.EnvironmentDefinition,
	definition configDefinition) (Config, []error) {

	if definition.Template == "" {
		return Config{}, []error{
			newDetailedDefinitionParserError(configId, context, environment, "missing property `template`"),
		}
	}

	template, err := template.LoadTemplate(fs, filepath.Join(context.Folder, definition.Template))

	var errors []error

	if err != nil {
		errors = append(errors, newDetailedDefinitionParserError(configId, context, environment, fmt.Sprintf("error while loading template: `%s`", err)))
	}

	parameters, configReferences, parameterErrors := parseParametersAndReferences(context, environment, configId,
		definition.Parameters)

	if parameterErrors != nil {
		errors = append(errors, parameterErrors...)
		parameters = make(map[string]parameter.Parameter)
		configReferences = make(map[string]coordinate.Coordinate)
	}

	var skipConfig bool = false

	if definition.Skip != nil {
		skip, err := parseSkip(context, environment, configId, definition.Skip)

		if err == nil {
			skipConfig = skip
		} else {
			errors = append(errors, err)
		}
	}

	if definition.Name != nil {
		name, err := parseParameter(context, environment, configId, NameParameter, definition.Name)
		if err != nil {
			errors = append(errors, err)
		} else {
			parameters[NameParameter] = name

			for _, ref := range name.GetReferences() {
				configReferences[ref.Config.String()] = ref.Config
			}
		}

	} else {
		errors = append(errors, newDetailedDefinitionParserError(configId, context, environment, "missing parameter `name`"))
	}

	if errors != nil {
		return Config{}, errors
	}

	return Config{
		Template: template,
		Coordinate: coordinate.Coordinate{
			Project: context.ProjectId,
			Api:     context.ApiId,
			Config:  configId,
		},
		Group:       environment.Group,
		Environment: environment.Name,
		Parameters:  parameters,
		References:  getReferenceSlice(configReferences),
		Skip:        skipConfig,
	}, nil
}

func parseSkip(context *ConfigLoaderContext, environment manifest.EnvironmentDefinition,
	configId string, param interface{}) (bool, error) {
	switch param := param.(type) {
	case bool:
		return param, nil
	case string:
		strVal := param

		switch strings.ToLower(strVal) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}

		return false, newDetailedDefinitionParserError(configId, context, environment,
			fmt.Sprintf("invalid value for `skip`: `%s`. only `true` and `false` are allowed", strVal))
	}

	return false, newDefinitionParserError(configId, context,
		"invalid value for `skip`: only bool or string types are allowed")
}

func getReferenceSlice(references map[string]coordinate.Coordinate) []coordinate.Coordinate {
	return maps.Values(references)
}

type References map[string]coordinate.Coordinate

func parseParametersAndReferences(context *ConfigLoaderContext, environment manifest.EnvironmentDefinition,
	configId string, parameterMap map[string]configParameter) (Parameters, References, []error) {

	parameters := make(map[string]parameter.Parameter)
	configReferences := make(map[string]coordinate.Coordinate)
	var errors []error

	for name, param := range parameterMap {
		if _, found := parameters[name]; found {
			errors = append(errors, newDefinitionParserError(configId, context, fmt.Sprintf("duplicate parameter `%s`", name)))
			continue
		}

		result, err := parseParameter(context, environment, configId, name, param)

		if err != nil {
			errors = append(errors, err)
			continue
		}

		parameters[name] = result

		for _, ref := range result.GetReferences() {
			configReferences[ref.Config.String()] = ref.Config
		}
	}

	if errors != nil {
		return nil, nil, errors
	}

	return parameters, configReferences, nil
}

func parseParameter(context *ConfigLoaderContext, environment manifest.EnvironmentDefinition,
	configId string, name string, param interface{}) (parameter.Parameter, error) {
	if name == IdParameter {
		return nil, newParameterDefinitionParserError(name, configId, context, environment,
			fmt.Sprintf("parameter name `%s` is not allowed (reserved)", IdParameter))
	}

	if val, ok := param.([]interface{}); ok {
		ref, err := arrayToReferenceParameter(context, environment, configId, name, val)

		if err != nil {
			return nil, err
		}

		return ref, nil
	} else if val, ok := param.(map[interface{}]interface{}); ok {
		parameterType := toString(val["type"])
		serDe, found := context.ParametersSerDe[parameterType]

		if !found {
			return nil, newParameterDefinitionParserError(name, configId, context, environment,
				fmt.Sprintf("unknown parameter type `%s`", parameterType))
		}

		return serDe.Deserializer(parameter.ParameterParserContext{
			Coordinate: coordinate.Coordinate{
				Project: context.ProjectId,
				Api:     context.ApiId,
				Config:  configId,
			},
			ParameterName: name,
			Value:         maps.ToStringMap(val),
		})
	}

	return valueParam.New(param), nil
}

// TODO come up with better way to handle this, as this is a hack
func arrayToReferenceParameter(context *ConfigLoaderContext, environment manifest.EnvironmentDefinition,
	configId string, parameterName string, arr []interface{}) (parameter.Parameter, error) {
	if len(arr) == 0 || len(arr) > 4 {
		return nil, newParameterDefinitionParserError(parameterName, configId, context, environment,
			fmt.Sprintf("short references must have between 1 and 4 elements. you provided `%d`", len(arr)))
	}

	project := context.ProjectId
	api := context.ApiId
	config := configId
	var property string

	switch len(arr) {
	case 1:
		property = toString(arr[0])
	case 2:
		config = toString(arr[0])
		property = toString(arr[1])
	case 3:
		api = toString(arr[0])
		config = toString(arr[1])
		property = toString(arr[2])
	case 4:
		project = toString(arr[0])
		api = toString(arr[1])
		config = toString(arr[2])
		property = toString(arr[3])
	}

	return refParam.New(project, api, config, property), nil
}

func toString(v interface{}) string {
	return fmt.Sprintf("%v", v)
}
