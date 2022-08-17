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

package runner

import (
	"errors"
	"fmt"
	"os"
	"strings"

	legacyDeploy "github.com/dynatrace-oss/dynatrace-monitoring-as-code/cmd/monaco/v1/deploy"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/cmd/monaco/convert"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/cmd/monaco/v2/delete"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/cmd/monaco/v2/deploy"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/log"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/version"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

var errWrongUsage = errors.New("")

var specificApi, environment, project []string
var environments, specificEnvironment, projects, workingDir, outputFolder, manifestName string
var verbose, dryRun, continueOnError bool

func Run() int {
	rootCmd := BuildCli(afero.NewOsFs())

	err := rootCmd.Execute()

	if err != nil {
		if err != errWrongUsage {
			// Log error if it wasn't a usage error
			log.Error("%v\n", err)
		}
		return 1
	}

	return 0
}

func BuildCli(fs afero.Fs) *cobra.Command {

	var rootCmd = &cobra.Command{
		Use:   "monaco <command>",
		Short: "Automates the deployment of Dynatrace Monitoring Configuration to one or multiple Dynatrace environments.",
		Long: `Tool used to deploy dynatrace configurations via the cli

		Examples:
		  Deploy a manifest
			monaco deploy service.yaml

		  Deploy a a specific environment within an manifest
			monaco deploy service.yaml -e dev`,

		PersistentPreRunE: configureLogging,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	downloadCommand := getDownloadCommand(fs)
	convertCommand := getConvertCommand(fs)
	deployCommand := getDeployCommand(fs)
	deleteCommand := getDeleteCommand(fs)

	if isEnvFlagEnabled("CONFIG_V1") {
		log.Warn("CONFIG_V1 environment var detected!")
		log.Warn("Please convert your config to v2 format, as the migration layer will get removed in one of the next releases!")
		deployCommand = getLegacyDeployCommand(fs)
	}

	rootCmd.AddCommand(downloadCommand)
	rootCmd.AddCommand(convertCommand)
	rootCmd.AddCommand(deployCommand)
	rootCmd.AddCommand(deleteCommand)

	return rootCmd
}

func configureLogging(cmd *cobra.Command, args []string) error {
	if verbose {
		log.Default().SetLevel(log.LevelDebug)
	}
	err := log.SetupLogging()
	if err != nil {
		return err
	}

	log.Info("Dynatrace Monitoring as Code v" + version.MonitoringAsCode)

	return nil
}

func getDeployCommand(fs afero.Fs) (deployCmd *cobra.Command) {
	deployCmd = &cobra.Command{
		Use:  "deploy manifest.yaml",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) == 0 {
				log.Error("deployment manifest path missing")
				return errWrongUsage
			}

			if len(args) > 1 {
				log.Error("too many arguments")
				return errWrongUsage
			}

			manifestName = args[0]

			if !strings.HasSuffix(manifestName, ".yaml") {
				log.Error("Wrong format for manifest file! expected a .yaml file")
				return errWrongUsage
			}

			return deploy.Deploy(fs, manifestName, environment, project, dryRun, continueOnError)
		},
	}
	deployCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print debug output")
	deployCmd.Flags().StringSliceVarP(&environment, "environment", "e", make([]string, 0), "Environment to deploy to")
	deployCmd.Flags().StringSliceVarP(&project, "project", "p", make([]string, 0), "Project configuration to deploy (also deploys any dependent configurations)")
	deployCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Switches to just validation instead of actual deployment")
	deployCmd.Flags().BoolVarP(&continueOnError, "continue-on-error", "c", false, "Proceed deployment even if config upload fails")
	return
}

func getDeleteCommand(fs afero.Fs) (deleteCmd *cobra.Command) {
	deleteCmd = &cobra.Command{
		Use:     "delete <manifest.yaml> <delete.yaml>",
		Short:   "Delete configurations defined in delete.yaml from the environments defined in the manifest",
		Example: "monaco delete manifest.yaml delete.yaml -e dev-environment",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) != 2 {
				log.Error("wrong number of arguments expected two")
				return errWrongUsage
			}

			manifestName = args[0]
			deleteFile := args[1]

			if !strings.HasSuffix(manifestName, ".yaml") {
				log.Error("Wrong format for manifest file! expected a .yaml file")
				return errWrongUsage
			}

			if !strings.HasSuffix(deleteFile, "delete.yaml") {
				log.Error("Wrong format for delete file! delete has to be named deletet.yaml")
				return errWrongUsage
			}

			return delete.Delete(fs, manifestName, deleteFile, environment)
		},
	}
	deleteCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print debug output")
	deleteCmd.Flags().StringSliceVarP(&environment, "environment", "e", make([]string, 0), "Deletes configuration only for specified envs. If not set, delete will be executed on all environments defined in manifest.")
	return deleteCmd
}

func getConvertCommand(fs afero.Fs) (convertCmd *cobra.Command) {
	convertCmd = &cobra.Command{
		Use:     "convert <environment.yaml> <config folder to convert>",
		Short:   "Convert v1 monaco configuration into v2 format",
		Example: "monaco convert environment.yaml my-v1-project -o my-v2-project",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {

			environmentsFile := args[0]
			workingDir := args[1]

			if !strings.HasSuffix(environmentsFile, ".yaml") {
				err := fmt.Errorf("wrong format for environment file! expected a .yaml file, but got %s", environmentsFile)
				return err
			}

			if !strings.HasSuffix(manifestName, ".yaml") {
				manifestName = manifestName + ".yaml"
			}

			if outputFolder == "{project folder}-v2" {
				outputFolder = workingDir + "-v2"
			}

			return convert.Convert(fs, workingDir, environmentsFile, outputFolder, manifestName)
		},
	}
	convertCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print debug output")
	convertCmd.Flags().StringVarP(&manifestName, "manifest", "m", "manifest.yaml", "Name of the manifest file to create")
	convertCmd.Flags().StringVarP(&outputFolder, "output-folder", "o", "{project folder}-v2", "Folder where to write converted config to")
	err := convertCmd.MarkFlagDirname("output-folder")
	if err != nil {
		log.Fatal("failed to setup CLI %v", err)
	}

	return convertCmd
}

func getLegacyDeployCommand(fs afero.Fs) (deployCmd *cobra.Command) {

	deployCmd = &cobra.Command{
		Use: "deploy",
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) > 1 {
				log.Error("too many arguments")
				return errWrongUsage
			}
			workingDir := "."
			if len(args) != 0 {
				workingDir = args[0]
			}

			return legacyDeploy.Deploy(fs, workingDir, environments, specificEnvironment, projects, dryRun, continueOnError)
		},
	}
	deployCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print debug output")
	deployCmd.Flags().StringVarP(&environments, "environments", "e", "", "Yaml file containing environment to deploy to")
	deployCmd.Flags().StringVarP(&projects, "project", "p", "", "Project configuration to deploy (also deploys any dependent configurations)")
	deployCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Switches to just validation instead of actual deployment")
	deployCmd.Flags().BoolVarP(&continueOnError, "continue-on-error", "c", false, "Proceed deployment even if config upload fails")
	deployCmd.MarkFlagFilename("environments")
	deployCmd.MarkFlagRequired("environments")
	return deployCmd
}

func getDownloadCommand(fs afero.Fs) (downloadCmd *cobra.Command) {

	downloadCmd = &cobra.Command{
		Use: "download",
		RunE: func(cmd *cobra.Command, args []string) error {
			var workingDir string

			if len(args) != 0 {
				workingDir = args[0]
			} else {
				workingDir = "."
			}

			return download.GetConfigsFilterByEnvironment(workingDir, fs, environments, specificEnvironment, specificApi)
		},
	}
	downloadCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print debug output")
	downloadCmd.Flags().StringVarP(&environments, "environments", "e", "", "Yaml file containing environment to download")
	downloadCmd.Flags().StringVarP(&specificEnvironment, "specific-environment", "s", "", "Specific environment (from list) to download")
	downloadCmd.Flags().StringSliceVarP(&specificApi, "specific-api", "a", make([]string, 0), "APIs to download")
	downloadCmd.MarkFlagFilename("environments")
	downloadCmd.MarkFlagRequired("environments")
	return downloadCmd

}

func isEnvFlagEnabled(env string) bool {
	val, ok := os.LookupEnv(env)
	return ok && val != "0"
}
