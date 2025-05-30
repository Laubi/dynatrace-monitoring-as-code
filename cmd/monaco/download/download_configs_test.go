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

package download

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/featureflags"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/testutils"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/api"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/client"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/client/dtclient"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/coordinate"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/download/classic"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/download/segment"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/download/settings"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/download/slo"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/manifest"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/project"
)

func TestDownloadConfigsBehaviour(t *testing.T) {
	var downloadOptions = downloadOptionsShared{
		environmentURL: manifest.URLDefinition{
			Type:  manifest.ValueURLType,
			Value: "testurl.com",
		},
		auth: manifest.Auth{
			Token: &manifest.AuthSecret{
				Name:  "TEST_TOKEN_VAR",
				Value: "test.token",
			},
		},
		outputFolder:           "folder",
		projectName:            "project",
		forceOverwriteManifest: false,
	}

	tests := []struct {
		name                      string
		givenOpts                 downloadConfigsOptions
		expectedConfigBehaviour   func(client *client.MockConfigClient)
		expectedSettingsBehaviour func(client *client.MockSettingsClient)
	}{
		{
			name: "Default opts: downloads Configs and Settings",
			givenOpts: downloadConfigsOptions{
				specificAPIs:          nil,
				specificSchemas:       nil,
				onlyAPIs:              false,
				onlySettings:          false,
				downloadOptionsShared: downloadOptions,
			},
			expectedConfigBehaviour: func(c *client.MockConfigClient) {
				c.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes().Return([]dtclient.Value{}, nil)
				c.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return([]byte("{}"), nil) // singleton configs are always attempted
			},
			expectedSettingsBehaviour: func(c *client.MockSettingsClient) {
				c.EXPECT().ListSchemas(gomock.Any()).Return(dtclient.SchemaList{}, nil)
				c.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return([]dtclient.DownloadSettingsObject{}, nil)
			},
		},
		{
			name: "Specific Settings: downloads defined Settings only",
			givenOpts: downloadConfigsOptions{
				specificAPIs:          nil,
				specificSchemas:       []string{"builtin:magic.secret"},
				onlyAPIs:              false,
				onlySettings:          false,
				downloadOptionsShared: downloadOptions,
			},
			expectedConfigBehaviour: func(c *client.MockConfigClient) {
				c.EXPECT().List(gomock.Any(), gomock.Any()).Times(0)
				c.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedSettingsBehaviour: func(c *client.MockSettingsClient) {
				c.EXPECT().ListSchemas(gomock.Any()).AnyTimes().Return(dtclient.SchemaList{{SchemaId: "builtin:magic.secret"}}, nil)
				c.EXPECT().GetSchema(gomock.Any(), gomock.Any()).AnyTimes().Return(dtclient.Schema{SchemaId: "builtin:magic.secret"}, nil)
				c.EXPECT().List(gomock.Any(), "builtin:magic.secret", gomock.Any()).AnyTimes().Return([]dtclient.DownloadSettingsObject{}, nil)
			},
		},
		{
			name: "Specific APIs: downloads defined APIs only",
			givenOpts: downloadConfigsOptions{
				specificAPIs:          []string{"alerting-profile"},
				specificSchemas:       nil,
				onlyAPIs:              false,
				onlySettings:          false,
				downloadOptionsShared: downloadOptions,
			},
			expectedConfigBehaviour: func(c *client.MockConfigClient) {
				c.EXPECT().List(gomock.Any(), api.NewAPIs()["alerting-profile"]).Return([]dtclient.Value{{Id: "42", Name: "profile"}}, nil)
				c.EXPECT().Get(gomock.Any(), gomock.Any(), "42").AnyTimes().Return([]byte("{}"), nil)
			},
			expectedSettingsBehaviour: func(c *client.MockSettingsClient) {
				c.EXPECT().ListSchemas(gomock.Any()).Times(0)
				c.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "Specific APIs and Settings: downloads defined APIs and Schemas",
			givenOpts: downloadConfigsOptions{
				specificAPIs:          []string{"alerting-profile"},
				specificSchemas:       []string{"builtin:magic.secret"},
				onlyAPIs:              false,
				onlySettings:          false,
				downloadOptionsShared: downloadOptions,
			},
			expectedConfigBehaviour: func(c *client.MockConfigClient) {
				c.EXPECT().List(gomock.Any(), api.NewAPIs()["alerting-profile"]).Return([]dtclient.Value{{Id: "42", Name: "profile"}}, nil)
				c.EXPECT().Get(gomock.Any(), gomock.Any(), "42").AnyTimes().Return([]byte("{}"), nil)
			},
			expectedSettingsBehaviour: func(c *client.MockSettingsClient) {
				c.EXPECT().ListSchemas(gomock.Any()).AnyTimes().Return(dtclient.SchemaList{{SchemaId: "builtin:magic.secret"}}, nil)
				c.EXPECT().GetSchema(gomock.Any(), gomock.Any()).AnyTimes().Return(dtclient.Schema{SchemaId: "builtin:magic.secret"}, nil)
				c.EXPECT().List(gomock.Any(), "builtin:magic.secret", gomock.Any()).AnyTimes().Return([]dtclient.DownloadSettingsObject{}, nil)
			},
		},
		{
			name: "Only APIs: downloads APIs only",
			givenOpts: downloadConfigsOptions{
				specificAPIs:          nil,
				specificSchemas:       nil,
				onlyAPIs:              true,
				onlySettings:          false,
				downloadOptionsShared: downloadOptions,
			},
			expectedConfigBehaviour: func(c *client.MockConfigClient) {
				c.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes().Return([]dtclient.Value{}, nil)
				c.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return([]byte("{}"), nil) // singleton configs are always attempted
			},
			expectedSettingsBehaviour: func(c *client.MockSettingsClient) {
				c.EXPECT().ListSchemas(gomock.Any()).Times(0)
				c.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "Only Settings: downloads Settings only",
			givenOpts: downloadConfigsOptions{
				specificAPIs:          nil,
				specificSchemas:       nil,
				onlyAPIs:              false,
				onlySettings:          true,
				downloadOptionsShared: downloadOptions,
			},
			expectedConfigBehaviour: func(c *client.MockConfigClient) {
				c.EXPECT().List(gomock.Any(), gomock.Any()).Times(0)
				c.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedSettingsBehaviour: func(c *client.MockSettingsClient) {
				c.EXPECT().ListSchemas(gomock.Any()).Return(dtclient.SchemaList{}, nil)
				c.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return([]dtclient.DownloadSettingsObject{}, nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			configClient := client.NewMockConfigClient(gomock.NewController(t))
			tt.expectedConfigBehaviour(configClient)

			settingsClient := client.NewMockSettingsClient(gomock.NewController(t))
			tt.expectedSettingsBehaviour(settingsClient)

			_, err := downloadConfigs(t.Context(), &client.ClientSet{ConfigClient: configClient, SettingsClient: settingsClient}, api.NewAPIs(), tt.givenOpts, defaultDownloadFn)
			assert.NoError(t, err)
		})
	}
}

func TestDownload_Options(t *testing.T) {
	type wantDownload struct {
		config, settings, bucket, automation, document, openpipeline, segment, slo bool
	}
	tests := []struct {
		name         string
		given        downloadConfigsOptions
		want         wantDownload
		featureFlags map[featureflags.FeatureFlag]bool
	}{
		{
			name: "download all if options are not limiting",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{Token: &manifest.AuthSecret{}, OAuth: &manifest.OAuth{}}, // OAuth and Token required to download whole config
				},
			},
			want: wantDownload{
				config:       true,
				settings:     true,
				bucket:       true,
				automation:   true,
				document:     true,
				openpipeline: true,
				segment:      true,
				slo:          true,
			},
		},
		{
			name: "only settings requested",
			given: downloadConfigsOptions{
				onlySettings: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{Token: &manifest.AuthSecret{}},
				}},
			want: wantDownload{settings: true},
		},
		{
			name: "specific settings requested",
			given: downloadConfigsOptions{
				specificSchemas: []string{"some:schema"},
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{Token: &manifest.AuthSecret{}},
				}},
			want: wantDownload{settings: true},
		},
		{
			name: "only documents requested",
			given: downloadConfigsOptions{
				onlyDocuments: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			want: wantDownload{document: true},
		},
		{
			name: "only buckets requested",
			given: downloadConfigsOptions{
				onlyBuckets: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			want: wantDownload{bucket: true},
		},
		{
			name: "only openpipeline requested",
			given: downloadConfigsOptions{
				onlyOpenPipeline: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			want: wantDownload{openpipeline: true},
		},
		{
			name: "only segment requested with FF on",
			given: downloadConfigsOptions{
				onlySegment: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			featureFlags: map[featureflags.FeatureFlag]bool{featureflags.Segments: true},
			want:         wantDownload{segment: true},
		},
		{
			name: "only segment requested with FF off",
			given: downloadConfigsOptions{
				onlySegment: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			featureFlags: map[featureflags.FeatureFlag]bool{featureflags.Segments: false},
			want:         wantDownload{},
		},
		{
			name: "only slo-v2 requested with FF on",
			given: downloadConfigsOptions{
				onlySLOV2: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			featureFlags: map[featureflags.FeatureFlag]bool{featureflags.ServiceLevelObjective: true},
			want:         wantDownload{slo: true},
		},
		{
			name: "only slo-v2 requested with FF off",
			given: downloadConfigsOptions{
				onlySLOV2: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				}},
			featureFlags: map[featureflags.FeatureFlag]bool{featureflags.ServiceLevelObjective: false},
			want:         wantDownload{},
		},
		{
			name: "only apis requested",
			given: downloadConfigsOptions{
				onlyAPIs: true,
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{Token: &manifest.AuthSecret{}},
				}},
			want: wantDownload{config: true},
		},
		{
			name: "specific config apis requested",
			given: downloadConfigsOptions{
				specificAPIs: []string{"alerting-profile"},
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{Token: &manifest.AuthSecret{}},
				}},
			want: wantDownload{config: true},
		},
		{
			name: "only automations requested",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{OAuth: &manifest.OAuth{}},
				},
				onlyAutomation: true,
			},
			want: wantDownload{automation: true},
		},
		{
			name: "specific APIs and schemas",
			given: downloadConfigsOptions{
				specificAPIs:    []string{"alerting-profile"},
				specificSchemas: []string{"some:schema"},
				downloadOptionsShared: downloadOptionsShared{
					auth: manifest.Auth{Token: &manifest.AuthSecret{}},
				}},
			want: wantDownload{config: true, settings: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for ff, v := range tt.featureFlags {
				t.Setenv(string(ff), strconv.FormatBool(v))
			}

			fn := downloadFn{
				classicDownload: func(context.Context, client.ConfigClient, string, api.APIs, classic.ContentFilters) (project.ConfigsPerType, error) {
					if !tt.want.config {
						t.Fatalf("classic config download was not meant to be called but was")
					}
					return nil, nil
				},
				settingsDownload: func(ctx context.Context, settingsClient client.SettingsClient, s string, filters settings.Filters, settingsType ...config.SettingsType) (project.ConfigsPerType, error) {
					if !tt.want.settings {
						t.Fatalf("settings download was not meant to be called but was")
					}
					return nil, nil
				},
				automationDownload: func(ctx context.Context, a client.AutomationClient, s string, automationType ...config.AutomationType) (project.ConfigsPerType, error) {
					if !tt.want.automation {
						t.Fatalf("automation download was not meant to be called but was")
					}
					return nil, nil
				},
				bucketDownload: func(ctx context.Context, b client.BucketClient, s string) (project.ConfigsPerType, error) {
					if !tt.want.bucket {
						t.Fatalf("automation download was not meant to be called but was")
					}
					return nil, nil
				},
				documentDownload: func(ctx context.Context, b client.DocumentClient, s string) (project.ConfigsPerType, error) {
					if !tt.want.document {
						t.Fatalf("document download was not meant to be called but was")
					}
					return nil, nil
				},
				openPipelineDownload: func(ctx context.Context, b client.OpenPipelineClient, s string) (project.ConfigsPerType, error) {
					if !tt.want.openpipeline {
						t.Fatalf("openpipeline download was not meant to be called but was")
					}
					return nil, nil
				},
				segmentDownload: func(ctx context.Context, b segment.DownloadSegmentClient, s string) (project.ConfigsPerType, error) {
					if !tt.want.segment {
						t.Fatalf("segment download was not meant to be called but was")
					}
					return nil, nil
				},
				sloDownload: func(ctx context.Context, b slo.DownloadSloClient, s string) (project.ConfigsPerType, error) {
					if !tt.want.slo {
						t.Fatalf("slo-v2 download was not meant to be called but was")
					}
					return nil, nil
				},
			}

			c := client.NewMockConfigClient(gomock.NewController(t))
			_, err := downloadConfigs(t.Context(), &client.ClientSet{ConfigClient: c}, api.NewAPIs(), tt.given, fn)
			assert.NoError(t, err)
		})
	}
}

func Test_shouldDownloadSettings(t *testing.T) {
	tests := []struct {
		name  string
		given downloadConfigsOptions
		want  bool
	}{
		{
			name: "true if not 'onlyAPIs'",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{},
				specificAPIs:          nil,
				specificSchemas:       nil,
				onlyAPIs:              false,
				onlySettings:          false,
			},
			want: true,
		},
		{
			name: "true if 'onlySettings'",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{},
				specificAPIs:          nil,
				specificSchemas:       nil,
				onlyAPIs:              false,
				onlySettings:          true,
			},
			want: true,
		},
		{
			name: "true if only 'specificSettings' defined",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{},
				specificAPIs:          nil,
				specificSchemas:       []string{"some-schema", "other-schema"},
				onlyAPIs:              false,
				onlySettings:          false,
			},
			want: true,
		},
		{
			name: "false if 'specificAPIs' defined",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{},
				specificAPIs:          []string{"some-api", "other-api"},
				specificSchemas:       nil,
				onlyAPIs:              false,
				onlySettings:          false,
			},
			want: false,
		},
		{
			name: "true if 'specificAPIs' and 'specificSchemas' defined",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{},
				specificAPIs:          []string{"some-api", "other-api"},
				specificSchemas:       []string{"some-schema", "other-schema"},
				onlyAPIs:              false,
				onlySettings:          false,
			},
			want: true,
		},
		{
			name: "false if 'specificAPIs' and onlyAPIs defined",
			given: downloadConfigsOptions{
				downloadOptionsShared: downloadOptionsShared{},
				specificAPIs:          []string{"some-api", "other-api"},
				specificSchemas:       nil,
				onlyAPIs:              true,
				onlySettings:          false,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, shouldDownloadSettings(tt.given), "shouldDownloadSettings(%v)", tt.given)
		})
	}
}

func TestDownloadConfigsExitsEarlyForUnknownSettingsSchema(t *testing.T) {

	givenOpts := downloadConfigsOptions{
		specificSchemas: []string{"UNKNOWN SCHEMA"},
		onlySettings:    false,
		downloadOptionsShared: downloadOptionsShared{
			environmentURL: manifest.URLDefinition{
				Type:  manifest.ValueURLType,
				Value: "testurl.com",
			},
			auth: manifest.Auth{
				Token: &manifest.AuthSecret{
					Name:  "TEST_TOKEN_VAR",
					Value: "test.token",
				},
			},
			outputFolder:           "folder",
			projectName:            "project",
			forceOverwriteManifest: false,
		},
	}

	c := client.NewMockSettingsClient(gomock.NewController(t))
	c.EXPECT().ListSchemas(gomock.Any()).Return(dtclient.SchemaList{{SchemaId: "builtin:some.schema"}}, nil)

	err := doDownloadConfigs(t.Context(), afero.NewMemMapFs(), &client.ClientSet{SettingsClient: c}, nil, givenOpts)
	assert.ErrorContains(t, err, "not known", "expected download to fail for unkown Settings Schema")
	c.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(0) // no downloads should even be attempted for unknown schema
}

func TestMapToAuth(t *testing.T) {
	t.Run("Best case scenario only with token", func(t *testing.T) {
		t.Setenv("TOKEN", "token_value")

		expected := &manifest.Auth{Token: &manifest.AuthSecret{Name: "TOKEN", Value: "token_value"}}

		actual, errs := auth{token: "TOKEN"}.mapToAuth()

		assert.Empty(t, errs)
		assert.Equal(t, expected, actual)
	})
	t.Run("Best case scenario with OAuth", func(t *testing.T) {
		t.Setenv("TOKEN", "token_value")
		t.Setenv("CLIENT_ID", "client_id_value")
		t.Setenv("CLIENT_SECRET", "client_secret_value")

		expected := &manifest.Auth{
			Token: &manifest.AuthSecret{Name: "TOKEN", Value: "token_value"},
			OAuth: &manifest.OAuth{
				ClientID:      manifest.AuthSecret{Name: "CLIENT_ID", Value: "client_id_value"},
				ClientSecret:  manifest.AuthSecret{Name: "CLIENT_SECRET", Value: "client_secret_value"},
				TokenEndpoint: nil,
			},
		}

		actual, errs := auth{
			token:        "TOKEN",
			clientID:     "CLIENT_ID",
			clientSecret: "CLIENT_SECRET",
		}.mapToAuth()

		assert.Empty(t, errs)
		assert.Equal(t, expected, actual)
	})
	t.Run("Token is missing", func(t *testing.T) {
		_, errs := auth{
			token: "TOKEN",
		}.mapToAuth()

		assert.Len(t, errs, 1)
		assert.Contains(t, errs, errors.New("the content of the environment variable \"TOKEN\" is not set"))
	})
	t.Run("Token is missing", func(t *testing.T) {
		_, errs := auth{
			token:        "TOKEN",
			clientID:     "CLIENT_ID",
			clientSecret: "CLIENT_SECRET",
		}.mapToAuth()

		assert.Len(t, errs, 3)
		assert.Contains(t, errs, errors.New("the content of the environment variable \"TOKEN\" is not set"))
		assert.Contains(t, errs, errors.New("the content of the environment variable \"CLIENT_ID\" is not set"))
		assert.Contains(t, errs, errors.New("the content of the environment variable \"CLIENT_SECRET\" is not set"))
	})
}

func TestDownloadConfigs_OnlyAutomationWithoutAutomationCredentials(t *testing.T) {
	opts := downloadConfigsOptions{
		onlyAutomation: true,
	}

	err := doDownloadConfigs(t.Context(), testutils.CreateTestFileSystem(), &client.ClientSet{}, nil, opts)
	assert.ErrorContains(t, err, "no OAuth credentials configured")
}

func TestDownloadConfigs_OnlySettings(t *testing.T) {
	opts := downloadConfigsOptions{
		onlySettings: true,
	}
	c := client.NewMockSettingsClient(gomock.NewController(t))
	c.EXPECT().ListSchemas(gomock.Any()).Return(dtclient.SchemaList{{SchemaId: "builtin:auto.schema"}}, nil)
	c.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return([]dtclient.DownloadSettingsObject{}, nil)
	err := doDownloadConfigs(t.Context(), testutils.CreateTestFileSystem(), &client.ClientSet{SettingsClient: c}, nil, opts)
	assert.NoError(t, err)
}

func Test_downloadConfigsOptions_valid(t *testing.T) {
	t.Run("no error for konwn api", func(t *testing.T) {
		given := downloadConfigsOptions{specificAPIs: []string{"alerting-profile"}}

		errs := given.valid()

		assert.Len(t, errs, 0)
	})
	t.Run("report error for unknown", func(t *testing.T) {
		given := downloadConfigsOptions{specificAPIs: []string{"unknown api"}}

		errs := given.valid()

		assert.Len(t, errs, 1)
		assert.ErrorContains(t, errs[0], "unknown api")
	})
}

func Test_copyConfigs(t *testing.T) {
	t.Run("Copy configs to empty", func(t *testing.T) {
		dest := project.ConfigsPerType{}
		copyConfigs(dest, project.ConfigsPerType{
			"dashboard": []config.Config{
				{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}}},
			"notebook": []config.Config{
				{Coordinate: coordinate.Coordinate{ConfigId: "notebook-1"}}},
		})

		assert.Len(t, dest, 2)

		assert.Contains(t, dest, "notebook")
		assert.EqualValues(t, dest["notebook"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "notebook-1"}}})

		assert.Contains(t, dest, "dashboard")
		assert.EqualValues(t, dest["dashboard"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}}})
	})

	t.Run("Copying configs of same type should merge", func(t *testing.T) {
		dest := project.ConfigsPerType{"dashboard": []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}},
		}}
		copyConfigs(dest, project.ConfigsPerType{"dashboard": []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-2"}},
		}})

		assert.Len(t, dest, 1)

		assert.Contains(t, dest, "dashboard")
		assert.EqualValues(t, dest["dashboard"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}},
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-2"}},
		})
	})

	t.Run("Copy configs of different types", func(t *testing.T) {
		dest := project.ConfigsPerType{"notebook": []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "notebook-1"}},
		}}
		copyConfigs(dest, project.ConfigsPerType{"dashboard": []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}}}})

		assert.Len(t, dest, 2)

		assert.Contains(t, dest, "notebook")
		assert.EqualValues(t, dest["notebook"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "notebook-1"}},
		})

		assert.Contains(t, dest, "dashboard")
		assert.EqualValues(t, dest["dashboard"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}},
		})
	})

	t.Run("Merge configs of same and different types", func(t *testing.T) {
		dest := project.ConfigsPerType{
			"notebook": []config.Config{
				{Coordinate: coordinate.Coordinate{ConfigId: "notebook-1"}},
			},
			"dashboard": []config.Config{
				{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}},
			},
		}

		copyConfigs(dest, project.ConfigsPerType{"dashboard": []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-2"}},
		}})

		assert.Len(t, dest, 2)

		assert.Contains(t, dest, "notebook")
		assert.EqualValues(t, dest["notebook"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "notebook-1"}},
		})

		assert.Contains(t, dest, "dashboard")
		assert.EqualValues(t, dest["dashboard"], []config.Config{
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-1"}},
			{Coordinate: coordinate.Coordinate{ConfigId: "dashboard-2"}},
		})
	})
}
