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

package writer

import (
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/manifest"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/manifest/internal/persistence"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/oauth2/endpoints"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/assert"
	"reflect"
	"sort"
	"testing"
)

func Test_toWriteableProjects(t *testing.T) {
	tests := []struct {
		name          string
		givenProjects map[string]manifest.ProjectDefinition
		wantResult    []persistence.Project
	}{
		{
			name: "creates_simple_projects",
			givenProjects: map[string]manifest.ProjectDefinition{
				"project_a": {
					Name: "a",
					Path: "projects/a",
				},
				"project_b": {
					Name: "b",
					Path: "projects/b",
				},
				"project_c": {
					Name: "c",
					Path: "projects/c",
				},
			},
			wantResult: []persistence.Project{
				{
					Name: "a",
					Path: "projects/a",
				},
				{
					Name: "b",
					Path: "projects/b",
				},
				{
					Name: "c",
					Path: "projects/c",
				},
			},
		},
		{
			"creates_grouping_projects",
			map[string]manifest.ProjectDefinition{
				"project_a": {
					Name: "projects.a",
					Path: "projects/a",
				},
				"project_b": {
					Name: "projects.b",
					Path: "projects/b",
				},
				"project_c": {
					Name: "projects.c",
					Path: "projects/c",
				},
			},
			[]persistence.Project{
				{
					Name: "projects",
					Path: "projects",
					Type: "grouping",
				},
			},
		},
		{
			name: "creates_mixed_projects",
			givenProjects: map[string]manifest.ProjectDefinition{
				"project_a": {
					Name: "projects.a",
					Path: "projects/a",
				},
				"project_b": {
					Name: "projects.b",
					Path: "projects/b",
				},
				"project_c": {
					Name: "projects.c",
					Path: "projects/c",
				},
				"project_alpha": {
					Name: "alpha",
					Path: "special_projects/alpha",
				},
				"nested_project_1": {
					Name: "nested.projects.deeply.grouped.one",
					Path: "nested/projects/deeply/grouped/one",
				},
				"nested_project_2": {
					Name: "nested.projects.deeply.grouped.two",
					Path: "nested/projects/deeply/grouped/two",
				},
			},
			wantResult: []persistence.Project{
				{
					Name: "alpha",
					Path: "special_projects/alpha",
				},
				{
					Name: "nested.projects.deeply.grouped",
					Path: "nested/projects/deeply/grouped",
					Type: "grouping",
				},
				{
					Name: "projects",
					Path: "projects",
					Type: "grouping",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult := toWriteableProjects(tt.givenProjects)
			assert.DeepEqual(t, gotResult, tt.wantResult, cmpopts.SortSlices(func(a, b persistence.Project) bool { return a.Name < b.Name }))
		})
	}
}

func Test_toWriteableEnvironmentGroups(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]manifest.EnvironmentDefinition
		wantResult []persistence.Group
	}{
		{
			name: "correctly transforms simple env groups",
			input: map[string]manifest.EnvironmentDefinition{
				"env1": {
					Name: "env1",
					URL: manifest.URLDefinition{
						Value: "www.an.Url",
					},
					Group: "group1",
					Auth: manifest.Auth{
						Token: manifest.AuthSecret{
							Name: "TokenTest",
						},
					},
				},
				"env2": {
					Name: "env2",
					URL: manifest.URLDefinition{
						Value: "www.an.Url",
					},
					Group: "group1",
					Auth: manifest.Auth{
						Token: manifest.AuthSecret{},
						OAuth: &manifest.OAuth{
							ClientID: manifest.AuthSecret{
								Name:  "client-id-key",
								Value: "client-id-val",
							},
							ClientSecret: manifest.AuthSecret{
								Name:  "client-secret-key",
								Value: "client-secret-val",
							},
							TokenEndpoint: &manifest.URLDefinition{
								Value: endpoints.Dynatrace.TokenURL,
								Type:  manifest.EnvironmentURLType,
								Name:  "ENV_TOKEN_ENDPOINT",
							},
						},
					},
				},
				"env2a": {
					Name: "env2",
					URL: manifest.URLDefinition{
						Value: "www.an.Url",
					},
					Group: "group1",
					Auth: manifest.Auth{
						Token: manifest.AuthSecret{},
						OAuth: &manifest.OAuth{
							ClientID: manifest.AuthSecret{
								Name:  "client-id-key",
								Value: "client-id-val",
							},
							ClientSecret: manifest.AuthSecret{
								Name:  "client-secret-key",
								Value: "client-secret-val",
							},
						},
					},
				},
				"env2b": {
					Name: "env2",
					URL: manifest.URLDefinition{
						Value: "www.an.Url",
					},
					Group: "group1",
					Auth: manifest.Auth{
						Token: manifest.AuthSecret{},
						OAuth: &manifest.OAuth{
							ClientID: manifest.AuthSecret{
								Name:  "client-id-key",
								Value: "client-id-val",
							},
							ClientSecret: manifest.AuthSecret{
								Name:  "client-secret-key",
								Value: "client-secret-val",
							},
							TokenEndpoint: &manifest.URLDefinition{
								Value: "http://custom.sso.token.endpoint",
								Type:  manifest.ValueURLType,
							},
						},
					},
				},
				"env3": {
					Name: "env3",
					URL: manifest.URLDefinition{
						Value: "www.an.Url",
					},
					Group: "group2",
					Auth: manifest.Auth{
						Token: manifest.AuthSecret{},
					},
				},
			},
			wantResult: []persistence.Group{
				{
					Name: "group1",
					Environments: []persistence.Environment{
						{
							Name: "env1",
							URL:  persistence.Url{Value: "www.an.Url"},
							Auth: persistence.Auth{
								Token: persistence.AuthSecret{
									Name: "TokenTest",
									Type: "environment",
								},
							},
						},
						{
							Name: "env2",
							URL:  persistence.Url{Value: "www.an.Url"},
							Auth: persistence.Auth{
								Token: persistence.AuthSecret{
									Name: "env2_TOKEN",
									Type: "environment",
								},
								OAuth: &persistence.OAuth{
									ClientID: persistence.AuthSecret{
										Type: persistence.TypeEnvironment,
										Name: "client-id-key",
									},
									ClientSecret: persistence.AuthSecret{
										Type: persistence.TypeEnvironment,
										Name: "client-secret-key",
									},
									TokenEndpoint: &persistence.Url{
										Type:  persistence.UrlTypeEnvironment,
										Value: "ENV_TOKEN_ENDPOINT",
									},
								},
							},
						},
						{
							Name: "env2a",
							URL:  persistence.Url{Value: "www.an.Url"},
							Auth: persistence.Auth{
								Token: persistence.AuthSecret{
									Name: "env2_TOKEN",
									Type: "environment",
								},
								OAuth: &persistence.OAuth{
									ClientID: persistence.AuthSecret{
										Type: persistence.TypeEnvironment,
										Name: "client-id-key",
									},
									ClientSecret: persistence.AuthSecret{
										Type: persistence.TypeEnvironment,
										Name: "client-secret-key",
									},
								},
							},
						},
						{
							Name: "env2b",
							URL:  persistence.Url{Value: "www.an.Url"},
							Auth: persistence.Auth{
								Token: persistence.AuthSecret{
									Name: "env2_TOKEN",
									Type: "environment",
								},
								OAuth: &persistence.OAuth{
									ClientID: persistence.AuthSecret{
										Type: persistence.TypeEnvironment,
										Name: "client-id-key",
									},
									ClientSecret: persistence.AuthSecret{
										Type: persistence.TypeEnvironment,
										Name: "client-secret-key",
									},
									TokenEndpoint: &persistence.Url{
										Value: "http://custom.sso.token.endpoint",
									},
								},
							},
						},
					},
				},
				{
					Name: "group2",
					Environments: []persistence.Environment{
						{
							Name: "env3",
							URL:  persistence.Url{Value: "www.an.Url"},
							Auth: persistence.Auth{
								Token: persistence.AuthSecret{
									Name: "env3_TOKEN",
									Type: "environment",
								},
							},
						},
					},
				},
			},
		},
		{
			"returns empty groups for empty env definition",
			map[string]manifest.EnvironmentDefinition{},
			[]persistence.Group{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotResult := toWriteableEnvironmentGroups(tt.input); gotResult != nil {
				assert.Equal(t, len(gotResult), len(tt.wantResult))

				// sort Entries sub-slices before checking equality of got and wanted group slices
				for _, g := range gotResult {
					sort.Slice(g.Environments, func(i, j int) bool {
						return g.Environments[i].Name < g.Environments[j].Name
					})
				}

				assert.DeepEqual(t,
					tt.wantResult,
					gotResult,
					cmpopts.SortSlices(func(a, b persistence.Group) bool { return a.Name < b.Name }),
				)
			}
		})
	}
}

func Test_toWriteableUrl(t *testing.T) {
	tests := []struct {
		name  string
		input manifest.EnvironmentDefinition
		want  persistence.Url
	}{
		{
			"correctly transforms env var Url",
			manifest.EnvironmentDefinition{
				Name: "NAME",
				URL: manifest.URLDefinition{
					Type:  manifest.EnvironmentURLType,
					Name:  "{{ .Env.VARIABLE }}",
					Value: "Some previously resolved value",
				},
				Group: "GROUP",
				Auth: manifest.Auth{
					Token: manifest.AuthSecret{},
				},
			},
			persistence.Url{
				Type:  persistence.UrlTypeEnvironment,
				Value: "{{ .Env.VARIABLE }}",
			},
		},
		{
			"correctly transforms value Url",
			manifest.EnvironmentDefinition{
				Name: "NAME",
				URL: manifest.URLDefinition{
					Type:  manifest.ValueURLType,
					Value: "www.an.Url",
				},
				Group: "GROUP",
				Auth: manifest.Auth{
					Token: manifest.AuthSecret{},
				},
			},
			persistence.Url{
				Value: "www.an.Url",
			},
		},
		{
			"defaults to value Url if no type is defined",
			manifest.EnvironmentDefinition{
				Name: "NAME",
				URL: manifest.URLDefinition{
					Value: "www.an.Url",
				},
				Group: "GROUP",
				Auth: manifest.Auth{
					Token: manifest.AuthSecret{},
				},
			},
			persistence.Url{
				Value: "www.an.Url",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toWriteableURL(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toWriteableURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_toWritableToken(t *testing.T) {
	tests := []struct {
		name  string
		input manifest.EnvironmentDefinition
		want  persistence.AuthSecret
	}{
		{
			"correctly transforms env var token",
			manifest.EnvironmentDefinition{
				Name:  "NAME",
				URL:   manifest.URLDefinition{},
				Group: "GROUP",
				Auth: manifest.Auth{
					Token: manifest.AuthSecret{Name: "VARIABLE"},
				},
			},
			persistence.AuthSecret{
				Name: "VARIABLE",
				Type: "environment",
			},
		},
		{
			"defaults to assumed token name if nothing is defined",
			manifest.EnvironmentDefinition{
				Name:  "NAME",
				URL:   manifest.URLDefinition{},
				Group: "GROUP",

				Auth: manifest.Auth{
					Token: manifest.AuthSecret{},
				},
			},
			persistence.AuthSecret{
				Name: "NAME_TOKEN",
				Type: "environment",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTokenSecret(tt.input.Auth, tt.input.Name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getTokenSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
